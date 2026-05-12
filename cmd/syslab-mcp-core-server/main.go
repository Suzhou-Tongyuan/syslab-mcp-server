package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syslab-mcp/internal/bridgeasset"
	"syslab-mcp/internal/config"
	"syslab-mcp/internal/discovery"
	"syslab-mcp/internal/mcpserver"
	"syslab-mcp/internal/session"
	"syslab-mcp/internal/skills"
	"syslab-mcp/internal/syslabenv"
	"syslab-mcp/internal/tydocs"
	"syslab-mcp/pkg/tools"
)

var version = "0.1.0"

func main() {
	cfg := parseFlags()
	logger := log.New(os.Stderr, "syslab-mcp: ", log.LstdFlags|log.Lmsgprefix)

	sess := session.NewManager(cfg, logger)
	defer sess.CloseAll()
	docCatalog := tydocs.NewCatalog(cfg.SyslabRoot, cfg.SyslabLauncher, cfg.HelpDocsRoot, logger)
	server := mcpserver.New(logger)
	var warmDocsOnce sync.Once
	skillContent, skillPath, _, skillErr := skills.LoadSkillFile(cfg.SkillFile)
	if skillErr != nil {
		logger.Printf("skills load failed: %v", skillErr)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		logger.Printf("interrupt received, shutting down Syslab sessions")
		sess.CloseAll()
		os.Exit(0)
	}()

	toolCatalog := tools.NewCatalog(sess, docCatalog, cfg.SkillFile, cfg.EnforceSkillPolicy)

	server.HandleInitialize(func(ctx context.Context, req mcpserver.Request) (any, error) {
		var initErr error
		warmDocsOnce.Do(func() {
			if err := docCatalog.Warmup(); err != nil {
				initErr = fmt.Errorf("Syslab Julia docs warmup failed: %w", err)
			}
		})
		if initErr != nil {
			return nil, initErr
		}
		if cfg.InitializeOnStartup {
			if err := sess.EnsureStarted(ctx); err != nil {
				return nil, err
			}
		}

		instructions, err := buildInitializeInstructions(docCatalog, skillContent, skillPath)
		if err != nil {
			return nil, err
		}

		return map[string]any{
			"protocolVersion": "2025-06-18",
			"serverInfo": map[string]any{
				"name":    "syslab-mcp-server",
				"version": version,
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"instructions": instructions,
		}, nil
	})

	server.HandleNotification("notifications/initialized", func(ctx context.Context, req mcpserver.Request) error {
		return nil
	})

	server.HandleMethod("ping", func(ctx context.Context, req mcpserver.Request) (any, error) {
		return map[string]any{}, nil
	})

	server.HandleMethod("tools/list", func(ctx context.Context, req mcpserver.Request) (any, error) {
		return map[string]any{
			"tools": toolCatalog.List(),
		}, nil
	})

	server.HandleMethod("tools/call", func(ctx context.Context, req mcpserver.Request) (any, error) {
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := req.UnmarshalParams(&params); err != nil {
			return nil, err
		}

		result, callErr := toolCatalog.Call(ctx, params.Name, params.Arguments)
		if callErr != nil {
			if result != "" {
				result += "\n\n"
			}
			result += "error:\n" + callErr.Error()
		}
		return map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": result,
				},
			},
			"isError": callErr != nil,
		}, callErr
	})

	if err := server.Serve(context.Background(), os.Stdin, os.Stdout); err != nil && !errors.Is(err, io.EOF) {
		logger.Printf("server stopped with error: %v", err)
	}
}

func parseFlags() config.Config {
	var cfg config.Config

	flag.StringVar(&cfg.SyslabLauncher, "syslab-launcher", "", "Absolute path to julia-ty.bat or julia-ty.sh")
	flag.StringVar(&cfg.SyslabRoot, "syslab-root", "", "Absolute path to the Syslab installation root (required)")
	flag.StringVar(&cfg.HelpDocsRoot, "help-docs-root", "", "Optional absolute path to the help docs projects root")
	flag.StringVar(&cfg.SkillsRoot, "skills-root", "", "Optional absolute path to the Syslab skill directory; defaults to the syslab-skills directory next to the MCP server directory")
	flag.StringVar(&cfg.SkillFile, "skill-file", "", "Optional absolute path to the Syslab skill markdown file; defaults to <skills-root>/SKILL.md")
	flag.StringVar(&cfg.JuliaRoot, "julia-root", "", "Absolute path to the Syslab Julia installation root")
	flag.StringVar(&cfg.MlangPath, "mlang-path", "", "Reserved path for mlang.bat or mlang.sh")
	flag.BoolVar(&cfg.InitializeOnStartup, "initialize-syslab-on-startup", false, "Start Syslab bridge when the MCP server starts")
	flag.BoolVar(&cfg.EnforceSkillPolicy, "enforce-skill-policy", true, "Require read_syslab_skill and detect_syslab_toolboxes before running Julia execution tools")
	flag.BoolVar(&cfg.PkgOffline, "pkg-offline", true, "Start Julia with package offline mode enabled by default")
	flag.StringVar(&cfg.InitialWorkingFolder, "initial-working-folder", "", "Initial working directory used when starting Syslab sessions")
	flag.StringVar(&cfg.SyslabDisplayMode, "syslab-display-mode", "desktop", "Execution mode. Current implementation treats nodesktop as Julia-only mode")
	flag.StringVar(&cfg.LogLevel, "log-level", "info", "Logging level")
	flag.BoolVar(&cfg.DisableTelemetry, "disable-telemetry", true, "Reserved compatibility flag")
	flag.Parse()

	if strings.TrimSpace(cfg.SyslabRoot) == "" {
		panic("--syslab-root is required")
	}

	cfg.InitialWorkingFolder = resolveInitialWorkingFolder(cfg.InitialWorkingFolder)

	if !filepath.IsAbs(cfg.SyslabRoot) {
		abs, err := filepath.Abs(cfg.SyslabRoot)
		if err == nil {
			cfg.SyslabRoot = abs
		}
	}
	if strings.TrimSpace(cfg.JuliaRoot) != "" && !filepath.IsAbs(cfg.JuliaRoot) {
		abs, err := filepath.Abs(cfg.JuliaRoot)
		if err == nil {
			cfg.JuliaRoot = abs
		}
	}

	root, err := discovery.ResolveSyslabRoot(cfg.SyslabRoot)
	if err != nil {
		panic(err)
	}
	cfg.SyslabRoot = root

	skillsRoot, err := skills.ResolveRoot(cfg.SkillsRoot)
	if err != nil {
		panic(err)
	}
	cfg.SkillsRoot = skillsRoot

	skillFile, err := skills.ResolvePrimarySkillFile(cfg.SkillFile, cfg.SkillsRoot)
	if err != nil {
		panic(err)
	}
	cfg.SkillFile = skillFile

	hasDefaultSyslabEnv, err := syslabenv.DefaultExists()
	if err != nil {
		panic(err)
	}
	cfg.SyslabDisplayMode = normalizeSyslabDisplayMode(cfg.SyslabDisplayMode, hasDefaultSyslabEnv)

	juliaRoot, err := discovery.ResolveJuliaRoot(cfg.JuliaRoot, cfg.SyslabRoot)
	if err != nil {
		panic(err)
	}
	cfg.JuliaRoot = juliaRoot

	launcher, err := discovery.ResolveSyslabLauncher(cfg.SyslabLauncher, cfg.JuliaRoot)
	if err != nil {
		panic(err)
	}
	cfg.SyslabLauncher = launcher

	if strings.TrimSpace(cfg.HelpDocsRoot) != "" && !filepath.IsAbs(cfg.HelpDocsRoot) {
		abs, err := filepath.Abs(cfg.HelpDocsRoot)
		if err == nil {
			cfg.HelpDocsRoot = abs
		}
	}
	cfg.BridgeScript = resolveBridgeScript()

	return cfg
}

func normalizeSyslabDisplayMode(mode string, hasDefaultSyslabEnv bool) string {
	normalized := strings.TrimSpace(mode)
	if normalized == "" {
		normalized = "desktop"
	}
	if !hasDefaultSyslabEnv {
		return "nodesktop"
	}
	return normalized
}

func resolveInitialWorkingFolder(dir string) string {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		wd, err := os.Getwd()
		if err == nil {
			return wd
		}
		return ""
	}
	if !filepath.IsAbs(trimmed) {
		abs, err := filepath.Abs(trimmed)
		if err == nil {
			return abs
		}
	}
	return trimmed
}

func resolveBridgeScript() string {
	embeddedPath, err := bridgeasset.Materialize()
	if err != nil {
		panic(fmt.Errorf("materialize embedded bridge script: %w", err))
	}
	return embeddedPath
}

func buildInitializeInstructions(docCatalog *tydocs.Catalog, skillContent string, skillPath string) (string, error) {
	basePolicy := "Syslab skill policy: first call read_syslab_skill to load the active Syslab skill. Before planning, writing, or executing Julia code, then call detect_syslab_toolboxes to inspect the current Syslab Julia environment. Prefer Ty libraries whenever they can satisfy the task. If Ty is not sufficient, prefer Julia libraries that are already installed in the current environment. Only suggest new community Julia packages when neither Ty nor the installed environment can satisfy the requirement. When API details are needed, call search_syslab_docs and read_syslab_doc before writing code."

	var sections []string
	if docCatalog == nil {
		sections = append(sections, basePolicy)
	} else {
		totalPackages, packagesWithDocs := docCatalog.Stats()
		sections = append(sections, fmt.Sprintf("%s Syslab Julia docs index is ready. Indexed doc packages: %d, packages with docs: %d.", basePolicy, totalPackages, packagesWithDocs))
	}

	if strings.TrimSpace(skillContent) != "" {
		if strings.TrimSpace(skillPath) != "" {
			sections = append(sections, fmt.Sprintf("Loaded built-in Syslab skill from %s.", skillPath))
		}
		sections = append(sections, "Built-in Syslab skill:\n"+skillContent)
	}

	return strings.Join(sections, "\n\n"), nil
}
