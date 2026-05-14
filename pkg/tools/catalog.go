package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syslab-mcp/internal/session"
	"syslab-mcp/internal/skills"
	"syslab-mcp/internal/tydocs"
)

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(context.Context, map[string]any) (string, error)
}

type Catalog struct {
	tools  map[string]Tool
	list   []map[string]any
	docs   *tydocs.Catalog
	policy *policyState
}

type policyState struct {
	mu                   sync.Mutex
	enforce              bool
	skillInspected       bool
	environmentInspected bool
}

func (p *policyState) markSkillInspected() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.skillInspected = true
}

func (p *policyState) requireSkillInspection(toolName string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.enforce || p.skillInspected {
		return nil
	}
	return fmt.Errorf("%s requires calling read_syslab_skill first so the active Syslab skill is loaded before using other Syslab MCP tools", toolName)
}

func (p *policyState) markEnvironmentInspected() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.environmentInspected = true
}

func (p *policyState) requireEnvironmentInspection(toolName string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.enforce || p.environmentInspected {
		return nil
	}
	return fmt.Errorf("%s requires calling read_syslab_skill first and then detect_syslab_toolboxes. Follow the Syslab skill policy: prefer Ty libraries first, then already-installed Julia libraries in the current environment, and only fall back to new community packages when neither is sufficient", toolName)
}

func NewCatalog(sess *session.Manager, docs *tydocs.Catalog, skillFile string, enforceSkillPolicy bool) *Catalog {
	c := &Catalog{
		tools: make(map[string]Tool),
		docs:  docs,
		policy: &policyState{
			enforce: enforceSkillPolicy,
		},
	}

	c.add(Tool{
		Name:        "detect_syslab_toolboxes",
		Description: "Returns Syslab version information from <syslab-root>/versionInfo, Julia depot information from julia-ty, installed Julia packages in the Syslab Julia environment including Ty packages, and discovered local docs paths for those packages. Call read_syslab_skill first to load the active Syslab skill, then call this tool before planning, writing, or executing Julia code so you can prefer Ty libraries and already-installed Julia packages in the current environment.",
		InputSchema: objectSchema(map[string]any{
			"include_all_packages": map[string]any{"type": "boolean"},
		}, nil),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			if err := c.policy.requireSkillInspection("detect_syslab_toolboxes"); err != nil {
				return "", err
			}
			includeAllPackages, err := optionalBool(args, "include_all_packages")
			if err != nil {
				return "", err
			}
			result, err := detectSyslabToolboxes(ctx, sess, docs, includeAllPackages)
			if err == nil {
				c.policy.markEnvironmentInspected()
			}
			return result, err
		},
	})

	c.add(Tool{
		Name:        "evaluate_julia_code",
		Description: "Evaluates a block of Julia code in the Syslab runtime and returns output plus the final result representation. Before using this tool, first call read_syslab_skill and then detect_syslab_toolboxes. Prefer Ty libraries whenever they can satisfy the task, then prefer Julia libraries that are already installed in the current environment, and only fall back to new community Julia packages when neither option is sufficient.",
		InputSchema: objectSchema(map[string]any{
			"code": map[string]any{"type": "string"},
		}, []string{"code"}),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			if err := c.policy.requireSkillInspection("evaluate_julia_code"); err != nil {
				return "", err
			}
			if err := c.policy.requireEnvironmentInspection("evaluate_julia_code"); err != nil {
				return "", err
			}
			code, err := requiredString(args, "code")
			if err != nil {
				return "", err
			}
			res, callErr := sess.Call(ctx, "evaluate", "", "", code)
			return formatBridgeResult("evaluate_julia_code", res), callErr
		},
	})

	c.add(Tool{
		Name:        "run_julia_file",
		Description: "Executes a Julia script file and returns the captured output. Before using this tool, first call read_syslab_skill and then detect_syslab_toolboxes. Prefer Ty libraries whenever they can satisfy the task, then prefer Julia libraries that are already installed in the current environment, and only fall back to new community Julia packages when neither option is sufficient.",
		InputSchema: objectSchema(map[string]any{
			"script_path": map[string]any{"type": "string"},
		}, []string{"script_path"}),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			if err := c.policy.requireSkillInspection("run_julia_file"); err != nil {
				return "", err
			}
			if err := c.policy.requireEnvironmentInspection("run_julia_file"); err != nil {
				return "", err
			}
			scriptPath, err := requiredString(args, "script_path")
			if err != nil {
				return "", err
			}
			scriptPath, err = normalizeJLFile(scriptPath)
			if err != nil {
				return "", err
			}

			res, callErr := sess.Call(ctx, "run_file", "", filepath.Dir(scriptPath), scriptPath)
			return formatBridgeResult("run_julia_file", res), callErr
		},
	})

	c.add(Tool{
		Name:        "restart_julia",
		Description: "Restarts the global Julia session.",
		InputSchema: objectSchema(map[string]any{
			"working_directory": map[string]any{"type": "string"},
		}, nil),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			workingDir, err := optionalString(args, "working_directory")
			if err != nil {
				return "", err
			}
			info, err := sess.Restart(ctx, workingDir)
			if err != nil {
				return "", err
			}
			data, marshalErr := json.MarshalIndent(map[string]any{
				"tool":    "restart_julia",
				"session": info,
			}, "", "  ")
			if marshalErr != nil {
				return "", marshalErr
			}
			return string(data), nil
		},
	})

	c.add(Tool{
		Name:        "read_syslab_skill",
		Description: "Reads the Syslab skill markdown file. Pass skill_path=\"default\" to read the default skill file, or pass an absolute skill markdown path to read a specific file.",
		InputSchema: objectSchema(map[string]any{
			"skill_path": map[string]any{"type": "string"},
		}, []string{"skill_path"}),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			requestedPath, err := optionalString(args, "skill_path")
			if err != nil {
				return "", err
			}
			resolvedSkillPath := skillFile
			if strings.EqualFold(strings.TrimSpace(requestedPath), "default") {
				resolvedSkillPath = skillFile
			} else if strings.TrimSpace(requestedPath) != "" {
				resolvedSkillPath = requestedPath
			}

			content, skillPath, truncated, err := skills.LoadSkillFile(resolvedSkillPath)
			if err != nil {
				return "", err
			}
			c.policy.markSkillInspected()
			data, err := json.MarshalIndent(map[string]any{
				"tool":       "read_syslab_skill",
				"skill_path": strings.TrimSpace(skillPath),
				"loaded":     strings.TrimSpace(content) != "",
				"content":    content,
				"truncated":  truncated,
			}, "", "  ")
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	})

	c.add(Tool{
		Name:        "search_syslab_docs",
		Description: "Searches the local indexed Syslab help docs.",
		InputSchema: objectSchema(map[string]any{
			"query":       map[string]any{"type": "string"},
			"package":     map[string]any{"type": "string"},
			"max_results": map[string]any{"type": "number"},
		}, []string{"query"}),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			if err := c.policy.requireSkillInspection("search_syslab_docs"); err != nil {
				return "", err
			}
			query, err := requiredString(args, "query")
			if err != nil {
				return "", err
			}
			packageName, _ := optionalString(args, "package")
			maxResults := 5
			if raw, ok := args["max_results"]; ok {
				if n, ok := raw.(float64); ok && int(n) > 0 {
					maxResults = int(n)
				}
			}
			if docs == nil {
				return "", fmt.Errorf("Syslab Julia docs catalog is not available")
			}
			result, err := docs.Search(query, packageName, maxResults)
			if err != nil {
				return "", err
			}
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	})

	c.add(Tool{
		Name:        "read_syslab_doc",
		Description: "Reads the full content of one indexed Syslab help document by path.",
		InputSchema: objectSchema(map[string]any{
			"doc_path": map[string]any{"type": "string"},
		}, []string{"doc_path"}),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			if err := c.policy.requireSkillInspection("read_syslab_doc"); err != nil {
				return "", err
			}
			docPath, err := requiredString(args, "doc_path")
			if err != nil {
				return "", err
			}
			if docs == nil {
				return "", fmt.Errorf("Syslab Julia docs catalog is not available")
			}
			result, err := docs.Read(docPath)
			if err != nil {
				return "", err
			}
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	})

	c.add(Tool{
		Name:        "map_matlab_functions_to_julia",
		Description: "Maps a batch of MATLAB function names to candidate functions in the Syslab Julia environment and related local docs. Use this for MATLAB code migration, function replacement, and MATLAB-to-Julia mapping, not for direct search over Syslab help docs.",
		InputSchema: objectSchema(map[string]any{
			"symbols": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"max_results_per_symbol": map[string]any{"type": "number"},
		}, []string{"symbols"}),
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			if err := c.policy.requireSkillInspection("map_matlab_functions_to_julia"); err != nil {
				return "", err
			}
			if docs == nil {
				return "", fmt.Errorf("Syslab Julia docs catalog is not available")
			}

			rawSymbols, ok := args["symbols"]
			if !ok {
				return "", fmt.Errorf("missing required argument: symbols")
			}
			items, ok := rawSymbols.([]any)
			if !ok {
				return "", fmt.Errorf("argument symbols must be an array of strings")
			}

			symbols := make([]string, 0, len(items))
			for _, item := range items {
				value, ok := item.(string)
				if !ok || strings.TrimSpace(value) == "" {
					return "", fmt.Errorf("argument symbols must be an array of non-empty strings")
				}
				symbols = append(symbols, value)
			}

			maxResultsPerSymbol := 3
			if raw, ok := args["max_results_per_symbol"]; ok {
				if n, ok := raw.(float64); ok && int(n) > 0 {
					maxResultsPerSymbol = int(n)
				}
			}

			result, err := docs.ResolveMatlabSymbols(symbols, maxResultsPerSymbol)
			if err != nil {
				return "", err
			}
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	})

	return c
}

func (c *Catalog) add(tool Tool) {
	c.tools[tool.Name] = tool
	c.list = append(c.list, map[string]any{
		"name":        tool.Name,
		"description": tool.Description,
		"inputSchema": tool.InputSchema,
	})
}

func (c *Catalog) List() []map[string]any {
	return c.list
}

func (c *Catalog) Call(ctx context.Context, name string, args map[string]any) (string, error) {
	tool, ok := c.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return tool.Handler(ctx, args)
}

func detectSyslabToolboxes(ctx context.Context, sess *session.Manager, docs *tydocs.Catalog, includeAllPackages bool) (string, error) {
	if sess != nil {
		if err := sess.EnsureRuntimeConfig(); err != nil {
			return "", err
		}
	}

	envInfo, err := tydocs.DiscoverInstalledPackages(docsRoot(docs, sess), launcherPath(docs, sess), includeAllPackages)
	if err != nil {
		return "", err
	}

	res, callErr := sess.Call(ctx, "detect_environment", "", "", "")
	docPackages := []tydocs.PackageDocs{}
	if docs != nil {
		if searchResult, err := docs.Search("Ty", "", 1); err == nil {
			docPackages = searchResult.Packages
		}
	}
	return formatToolboxDetection(envInfo, sess, res, docPackages), callErr
}

func formatToolboxDetection(envInfo tydocs.EnvironmentInfo, sess *session.Manager, res session.BridgeResult, docPackages []tydocs.PackageDocs) string {
	metadata := parseDetectEnvironmentOutput(res.Result)
	toolboxes := make([]toolboxInfo, 0, len(envInfo.Packages))
	docsByName := make(map[string]tydocs.PackageDocs, len(docPackages))
	for _, pkg := range docPackages {
		docsByName[pkg.Name] = pkg
	}
	for _, pkg := range envInfo.Packages {
		item := toolboxInfo{Name: pkg.Name, Version: pkg.Version, Path: pkg.PackagePath}
		if docPkg, ok := docsByName[pkg.Name]; ok {
			item.DocsPath = docPkg.DocsPath
			item.HasDocs = docPkg.HasDocs
		}
		toolboxes = append(toolboxes, item)
	}

	body := map[string]any{
		"tool":                "detect_syslab_toolboxes",
		"syslab_env_file":     envInfo.SyslabEnvFile,
		"julia_launcher_file": envInfo.LauncherFile,
		"syslab_version_file": envInfo.SyslabVersionFile,
		"syslab_version":      envInfo.SyslabVersion,
		"julia_home":          strings.TrimSpace(envInfo.Env.Values["JULIA_HOME"]),
		"julia_depot_path":    strings.TrimSpace(envInfo.Env.Values["JULIA_DEPOT_PATH"]),
		"julia_plugin_path":   strings.TrimSpace(envInfo.Env.Values["JULIA_PLUGIN_PATH"]),
		"ty_conda3":           strings.TrimSpace(envInfo.Env.Values["TY_CONDA3"]),
		"python":              strings.TrimSpace(envInfo.Env.Values["PYTHON"]),
		"syslab_launcher":     sess.LauncherPath(),
		"session_pid":         strings.TrimSpace(metadata["session_pid"]),
		"julia_version":       firstNonEmpty(strings.TrimSpace(metadata["julia_version"]), envInfo.ManifestJuliaVersion),
		"bindir":              strings.TrimSpace(metadata["bindir"]),
		"depot_path":          strings.TrimSpace(metadata["depot_path"]),
		"active_project":      envInfo.ActiveProject,
		"manifest_file":       envInfo.ManifestFile,
		"session_reuse":       true,
		"toolboxes":           toolboxes,
	}
	if strings.TrimSpace(res.Stderr) != "" {
		body["stderr"] = res.Stderr
	}
	if strings.TrimSpace(res.ErrorType) != "" || strings.TrimSpace(res.ErrorMsg) != "" {
		body["error"] = map[string]string{
			"type":    res.ErrorType,
			"message": res.ErrorMsg,
		}
	}

	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"tool":"detect_syslab_toolboxes","error":{"message":%q}}`, err.Error())
	}
	return string(data)
}

func docsRoot(docs *tydocs.Catalog, sess *session.Manager) string {
	if docs != nil {
		return docs.SyslabRoot()
	}
	return ""
}

func launcherPath(docs *tydocs.Catalog, sess *session.Manager) string {
	if docs != nil && strings.TrimSpace(docs.LauncherPath()) != "" {
		return docs.LauncherPath()
	}
	if sess != nil {
		return sess.LauncherPath()
	}
	return ""
}

type toolboxInfo struct {
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
	Path     string `json:"path,omitempty"`
	DocsPath string `json:"docs_path,omitempty"`
	HasDocs  bool   `json:"has_docs"`
}

func parseDetectEnvironmentOutput(text string) map[string]string {
	metadata := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		metadata[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return metadata
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

func requiredString(args map[string]any, key string) (string, error) {
	value, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required argument: %s", key)
	}
	s, ok := value.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("argument %s must be a non-empty string", key)
	}
	return s, nil
}

func optionalString(args map[string]any, key string) (string, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return "", nil
	}
	s, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("argument %s must be a string", key)
	}
	return s, nil
}

func optionalBool(args map[string]any, key string) (bool, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return false, nil
	}
	b, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("argument %s must be a boolean", key)
	}
	return b, nil
}

func normalizeJLFile(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if strings.ToLower(filepath.Ext(abs)) != ".jl" {
		return "", fmt.Errorf("script_path must point to a .jl file")
	}
	if _, err := os.Stat(abs); err != nil {
		return "", err
	}
	return abs, nil
}

func formatBridgeResult(toolName string, res session.BridgeResult) string {
	var b strings.Builder
	b.WriteString("tool: ")
	b.WriteString(toolName)
	b.WriteString("\n")

	if strings.TrimSpace(res.Stdout) != "" {
		b.WriteString("\nstdout:\n")
		b.WriteString(res.Stdout)
		if !strings.HasSuffix(res.Stdout, "\n") {
			b.WriteString("\n")
		}
	}

	if strings.TrimSpace(res.Stderr) != "" {
		b.WriteString("\nstderr:\n")
		b.WriteString(res.Stderr)
		if !strings.HasSuffix(res.Stderr, "\n") {
			b.WriteString("\n")
		}
	}

	if strings.TrimSpace(res.Result) != "" {
		b.WriteString("\nresult:\n")
		b.WriteString(res.Result)
		if !strings.HasSuffix(res.Result, "\n") {
			b.WriteString("\n")
		}
	}

	if strings.TrimSpace(res.ErrorType) != "" || strings.TrimSpace(res.ErrorMsg) != "" {
		b.WriteString("\nerror:\n")
		if res.ErrorType != "" {
			b.WriteString(res.ErrorType)
			b.WriteString(": ")
		}
		b.WriteString(res.ErrorMsg)
		b.WriteString("\n")
		if res.Stack != "" {
			b.WriteString("\nstacktrace:\n")
			b.WriteString(res.Stack)
			if !strings.HasSuffix(res.Stack, "\n") {
				b.WriteString("\n")
			}
		}
	}

	return strings.TrimSpace(b.String())
}
