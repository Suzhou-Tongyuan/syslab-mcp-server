package config

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
	BridgeScript         string
	LogLevel             string
	DisableTelemetry     bool
}
