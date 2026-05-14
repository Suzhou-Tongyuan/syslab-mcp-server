package config

import "time"

type Config struct {
	SyslabLauncher       string
	SyslabRoot           string
	HelpDocsRoot         string
	SkillsRoot           string
	SkillFile            string
	JuliaRoot            string
	MlangPath            string
	InitializeOnStartup  bool
	EnforceSkillPolicy   bool
	PkgOffline           bool
	InitialWorkingFolder string
	SyslabDisplayMode    string
	DesktopStartupTimeout time.Duration
	DesktopReadyTimeout   time.Duration
	DesktopAttachTimeout  time.Duration
	DesktopREPLTimeout    time.Duration
	DesktopControlTimeout time.Duration
	BridgeScript         string
	LogLevel             string
	DisableTelemetry     bool
}
