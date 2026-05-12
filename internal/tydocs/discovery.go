package tydocs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"syslab-mcp/internal/syslabenv"
)

type EnvironmentInfo struct {
	Env                  syslabenv.Env
	SyslabEnvFile        string
	LauncherFile         string
	SyslabVersion        string
	SyslabVersionFile    string
	ManifestFile         string
	ActiveProject        string
	ManifestJuliaVersion string
	Packages             []PackageInfo
}

type PackageInfo struct {
	Name        string
	Version     string
	PackagePath string
}

var manifestSectionPattern = regexp.MustCompile(`^\[\[deps\.(.+)\]\]$`)
var juliaVersionPattern = regexp.MustCompile(`julia-(\d+\.\d+)(?:\.\d+)?`)

type buildInfo struct {
	Version string `json:"version"`
}

func DiscoverInstalledPackages(syslabRoot string, launcherPath string) (EnvironmentInfo, error) {
	launcherEnv, err := syslabenv.LoadFromLauncher(launcherPath)
	if err != nil {
		return EnvironmentInfo{}, err
	}
	syslabEnv, err := syslabenv.LoadDefaultIfExists()
	if err != nil {
		return EnvironmentInfo{}, err
	}
	env := syslabenv.Merge(syslabEnv, launcherEnv)
	syslabVersion, syslabVersionFile, err := detectSyslabVersion(syslabRoot)
	if err != nil {
		return EnvironmentInfo{}, err
	}
	manifestFile, activeProject, manifestJuliaVersion, packages, err := detectPackagesFromGlobalManifest(env)
	if err != nil {
		return EnvironmentInfo{}, err
	}
	return EnvironmentInfo{
		Env:                  env,
		SyslabEnvFile:        syslabEnv.Path,
		LauncherFile:         launcherEnv.Path,
		SyslabVersion:        syslabVersion,
		SyslabVersionFile:    syslabVersionFile,
		ManifestFile:         manifestFile,
		ActiveProject:        activeProject,
		ManifestJuliaVersion: manifestJuliaVersion,
		Packages:             packages,
	}, nil
}

func detectSyslabVersion(syslabRoot string) (string, string, error) {
	buildInfoPath := filepath.Join(syslabRoot, "versionInfo", "build_info.json")
	data, err := os.ReadFile(buildInfoPath)
	if err != nil {
		return "", "", fmt.Errorf("read Syslab version info: %w", err)
	}

	var info buildInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return "", "", fmt.Errorf("parse Syslab version info %q: %w", buildInfoPath, err)
	}
	version := strings.TrimSpace(info.Version)
	if version == "" {
		return "", "", fmt.Errorf("Syslab version info %q does not contain a version", buildInfoPath)
	}
	return version, buildInfoPath, nil
}

func detectPackagesFromGlobalManifest(env syslabenv.Env) (manifestFile string, activeProject string, manifestJuliaVersion string, packages []PackageInfo, err error) {
	depotPath := strings.TrimSpace(env.Values["JULIA_DEPOT_PATH"])
	if depotPath == "" {
		fallbackDepot, ok, err := syslabenv.EnvDepotPathIfExists()
		if err != nil {
			return "", "", "", nil, err
		}
		if ok {
			depotPath = fallbackDepot
			env.Values["JULIA_DEPOT_PATH"] = fallbackDepot
		}
	}
	if depotPath == "" {
		return "", "", "", nil, fmt.Errorf("JULIA_DEPOT_PATH not found in syslab env, launcher, or environment variable")
	}

	// Try each depot path until we find one with a valid global environment
	depotPaths := splitDepotPaths(depotPath)
	var lastErr error
	for _, dp := range depotPaths {
		dp = strings.TrimSpace(dp)
		if dp == "" {
			continue
		}
		envDir, resolveErr := resolveGlobalEnvironmentDir(dp, env)
		if resolveErr != nil {
			lastErr = resolveErr
			continue
		}
		// Found a valid depot path, update the env value
		env.Values["JULIA_DEPOT_PATH"] = dp
		activeProject = filepath.Join(envDir, "Project.toml")
		manifestFile = filepath.Join(envDir, "Manifest.toml")

		file, openErr := os.Open(manifestFile)
		if openErr != nil {
			lastErr = fmt.Errorf("open manifest file: %w", openErr)
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

		candidateJuliaVersion := ""
		candidatePackages := make([]PackageInfo, 0)
		var current *PackageInfo
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if candidateJuliaVersion == "" && strings.HasPrefix(line, "julia_version = ") {
				candidateJuliaVersion = trimManifestString(line)
				continue
			}
			if matches := manifestSectionPattern.FindStringSubmatch(line); len(matches) == 2 {
				if current != nil && current.Name != "" && IsTargetPackage(current.Name) {
					candidatePackages = append(candidatePackages, *current)
				}
				current = &PackageInfo{Name: strings.TrimSpace(matches[1])}
				continue
			}
			if current == nil || strings.HasPrefix(line, "[") {
				continue
			}
			if strings.HasPrefix(line, "version = ") {
				current.Version = trimManifestString(line)
				continue
			}
			if strings.HasPrefix(line, "path = ") {
				current.PackagePath = trimManifestString(line)
				continue
			}
		}
		if err := scanner.Err(); err != nil {
			lastErr = fmt.Errorf("scan manifest file: %w", err)
			continue
		}
		if current != nil && current.Name != "" && IsTargetPackage(current.Name) {
			candidatePackages = append(candidatePackages, *current)
		}
		manifestJuliaVersion = candidateJuliaVersion
		packages = candidatePackages
		return manifestFile, activeProject, manifestJuliaVersion, packages, nil
	}

	if lastErr != nil {
		return "", "", "", nil, lastErr
	}
	return "", "", "", nil, fmt.Errorf("no valid Julia global environment found in depot paths: %s", depotPath)
}

// splitDepotPaths splits a depot path string by the platform path-list separator.
func splitDepotPaths(paths string) []string {
	return strings.Split(paths, string(os.PathListSeparator))
}

func IsTargetPackage(name string) bool {
	return strings.HasPrefix(name, "Ty") || name == "Syslab"
}

func detectJuliaEnvVersion(env syslabenv.Env) string {
	if home := strings.TrimSpace(env.Values["JULIA_HOME"]); home != "" {
		if matches := juliaVersionPattern.FindStringSubmatch(strings.ReplaceAll(home, "\\", "/")); len(matches) == 2 {
			return matches[1]
		}
	}
	return ""
}

func resolveGlobalEnvironmentDir(depotPath string, env syslabenv.Env) (string, error) {
	envRoot := filepath.Join(depotPath, "environments")
	entries, err := os.ReadDir(envRoot)
	if err != nil {
		return "", fmt.Errorf("read Julia environments: %w", err)
	}

	type envCandidate struct {
		name  string
		path  string
		major int
		minor int
	}

	candidates := make([]envCandidate, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		major, minor, ok := parseJuliaEnvDir(name)
		if !ok {
			continue
		}
		path := filepath.Join(envRoot, name)
		if _, err := os.Stat(filepath.Join(path, "Manifest.toml")); err != nil {
			continue
		}
		candidates = append(candidates, envCandidate{name: name, path: path, major: major, minor: minor})
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no Julia global environment with Manifest.toml found under %s", envRoot)
	}

	if version := detectJuliaEnvVersion(env); version != "" {
		target := "v" + version
		for _, candidate := range candidates {
			if candidate.name == target {
				return candidate.path, nil
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].major == candidates[j].major {
			return candidates[i].minor > candidates[j].minor
		}
		return candidates[i].major > candidates[j].major
	})
	return candidates[0].path, nil
}

func parseJuliaEnvDir(name string) (major int, minor int, ok bool) {
	if !strings.HasPrefix(name, "v") {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimPrefix(name, "v"), ".")
	if len(parts) != 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

func trimManifestString(line string) string {
	_, value, ok := strings.Cut(line, "=")
	if !ok {
		return ""
	}
	return strings.Trim(strings.TrimSpace(value), `"`)
}
