package syslabenv

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	windowsDepotPattern = regexp.MustCompile(`(?im)^\s*set\s+JULIA_DEPOT_PATH=(.+?)\s*$`)
	posixDepotPattern   = regexp.MustCompile(`(?m)^\s*export\s+JULIA_DEPOT_PATH=(.+?)\s*$`)
)

type Env struct {
	Path   string
	Values map[string]string
}

func Load(path string) (Env, error) {
	file, err := os.Open(path)
	if err != nil {
		return Env{}, fmt.Errorf("open syslab env file: %w", err)
	}
	defer file.Close()

	values := make(map[string]string)
	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section != "" && !strings.EqualFold(section, "Syslab") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return Env{}, fmt.Errorf("read syslab env file: %w", err)
	}

	expanded := make(map[string]string, len(values))
	for key := range values {
		expanded[key] = expand(key, values, map[string]bool{})
	}

	return Env{Path: path, Values: expanded}, nil
}

func LoadDefaultIfExists() (Env, error) {
	path, err := DefaultPath()
	if err != nil {
		return Env{}, err
	}
	env, err := Load(path)
	if err == nil {
		return env, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return Env{Values: map[string]string{}}, nil
	}
	return Env{}, err
}

func DefaultPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(homeDir, ".syslab", "syslab-env.ini"), nil
}

func DefaultExists() (bool, error) {
	path, err := DefaultPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat syslab env file: %w", err)
}

func LoadFromLauncher(launcherPath string) (Env, error) {
	content, err := os.ReadFile(launcherPath)
	if err != nil {
		return Env{}, fmt.Errorf("read launcher file: %w", err)
	}

	values := map[string]string{}
	switch strings.ToLower(filepath.Ext(launcherPath)) {
	case ".bat", ".cmd":
		if depot, ok := parseWindowsDepotPath(launcherPath, string(content)); ok {
			values["JULIA_DEPOT_PATH"] = depot
		}
	case ".sh":
		if depot, ok := parsePosixDepotPath(launcherPath, string(content)); ok {
			values["JULIA_DEPOT_PATH"] = depot
		}
	default:
		return Env{}, fmt.Errorf("unsupported launcher type: %s", launcherPath)
	}

	return Env{Path: launcherPath, Values: values}, nil
}

func Merge(primary Env, fallback Env) Env {
	merged := Env{
		Path:   primary.Path,
		Values: make(map[string]string, len(primary.Values)+len(fallback.Values)),
	}
	if merged.Path == "" {
		merged.Path = fallback.Path
	}
	for key, value := range fallback.Values {
		merged.Values[key] = value
	}
	for key, value := range primary.Values {
		merged.Values[key] = value
	}
	return merged
}

// EnvDepotPathIfExists reads JULIA_DEPOT_PATH environment variable and returns
// the first existing depot path. The environment variable may contain multiple
// paths separated by the platform path-list separator.
func EnvDepotPathIfExists() (string, bool, error) {
	depotEnv := strings.TrimSpace(os.Getenv("JULIA_DEPOT_PATH"))
	if depotEnv == "" {
		return "", false, nil
	}
	return FirstExistingDepotPath(depotEnv)
}

// splitDepotPaths splits a depot path string by the platform path-list separator.
func splitDepotPaths(paths string) []string {
	return strings.Split(paths, string(os.PathListSeparator))
}

func parseWindowsDepotPath(launcherPath, content string) (string, bool) {
	if override := strings.TrimSpace(os.Getenv("TY_DEPOT_PATH")); override != "" {
		return normalizeDepotPathList(override, ";"), true
	}
	matches := windowsDepotPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return "", false
	}
	raw := strings.TrimSpace(matches[0][1])
	raw = strings.ReplaceAll(raw, "%~dp0", ensureTrailingSeparator(filepath.Dir(launcherPath)))
	raw = strings.ReplaceAll(raw, "/", string(filepath.Separator))
	return normalizeDepotPathList(raw, ";"), true
}

func parsePosixDepotPath(launcherPath, content string) (string, bool) {
	if override := strings.TrimSpace(os.Getenv("TY_DEPOT_PATH")); override != "" {
		return normalizeDepotPathList(override, ":"), true
	}
	matches := posixDepotPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return "", false
	}
	raw := strings.Trim(strings.TrimSpace(matches[0][1]), `"`)
	base := filepath.Dir(launcherPath)
	raw = strings.ReplaceAll(raw, "$script_dir", base)
	raw = strings.ReplaceAll(raw, "${script_dir}", base)
	return normalizeDepotPathList(raw, ":"), true
}

func normalizeDepotPathList(paths string, separator string) string {
	parts := strings.Split(paths, separator)
	normalized := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		normalized = append(normalized, filepath.Clean(p))
	}
	return strings.Join(normalized, separator)
}

func FirstExistingDepotPath(paths string) (string, bool, error) {
	for _, p := range splitDepotPaths(paths) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		info, err := os.Stat(p)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", false, fmt.Errorf("stat depot path %q: %w", p, err)
		}
		if info.IsDir() {
			return p, true, nil
		}
	}
	return "", false, nil
}

func ensureTrailingSeparator(path string) string {
	cleaned := filepath.Clean(path)
	if strings.HasSuffix(cleaned, string(filepath.Separator)) {
		return cleaned
	}
	return cleaned + string(filepath.Separator)
}

func expand(key string, values map[string]string, visiting map[string]bool) string {
	if visiting[key] {
		if envValue, ok := os.LookupEnv(key); ok {
			return envValue
		}
		return ""
	}
	visiting[key] = true
	value := values[key]
	for {
		start := strings.Index(value, "${")
		if start < 0 {
			break
		}
		end := strings.Index(value[start+2:], "}")
		if end < 0 {
			break
		}
		end += start + 2
		ref := value[start+2 : end]
		replacement := ""
		if _, ok := values[ref]; ok {
			replacement = expand(ref, values, visiting)
		} else if envValue, ok := os.LookupEnv(ref); ok {
			replacement = envValue
		}
		value = value[:start] + replacement + value[end+1:]
	}
	delete(visiting, key)
	return filepath.Clean(strings.ReplaceAll(value, "/", string(filepath.Separator)))
}
