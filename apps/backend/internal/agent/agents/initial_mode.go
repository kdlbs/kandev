package agents

// InitialModeChannel names how an agent accepts the permission mode it should
// start in. Only agents with a verified wire contract declare one.
type InitialModeChannel string

const (
	// InitialModeChannelNone means the agent offers no way to configure a mode
	// before its process starts. Kandev keeps the post-creation mode switch and
	// records the mode as best-effort rather than as delivered.
	InitialModeChannelNone InitialModeChannel = ""

	// InitialModeChannelSettingsFile means the agent resolves its start mode
	// from a settings file inside a configuration directory it reads from the
	// environment. Kandev materializes a per-session directory and points the
	// agent at it.
	InitialModeChannelSettingsFile InitialModeChannel = "settings_file"
)

// InitialModeDelivery describes an agent's start-mode channel.
//
// It exists because applying a mode after session/new is not equivalent to
// starting the process in it: the mode reaches the agent's instruction layer
// while the process keeps enforcing what it was launched with.
type InitialModeDelivery struct {
	Channel InitialModeChannel

	// ConfigDirEnvVar is the environment variable naming the agent's
	// configuration directory, for InitialModeChannelSettingsFile.
	ConfigDirEnvVar string

	// DefaultConfigDirTemplate locates the user's existing configuration
	// directory when ConfigDirEnvVar is unset in the environment. "{home}" is
	// replaced with the user's home directory.
	DefaultConfigDirTemplate string

	// SettingsFileName is the file inside the configuration directory that
	// carries the mode.
	SettingsFileName string

	// ModeKeyPath is the nested JSON key path holding the mode value, for
	// example ["permissions", "defaultMode"].
	ModeKeyPath []string

	// ModeValues maps a Kandev session mode to the value this agent expects in
	// its settings file. A mode absent from the map cannot be delivered through
	// this channel.
	ModeValues map[string]string

	// SandboxEnv is set for executors whose process identity would otherwise
	// make a permissive mode unavailable. The bundled Claude bridge disables
	// bypassPermissions for a root process unless IS_SANDBOX is set, and a
	// container is exactly the isolation that escape hatch exists for.
	SandboxEnv map[string]string
}

// Delivers reports whether this declaration can carry the given session mode.
func (d InitialModeDelivery) Delivers(mode string) bool {
	if d.Channel != InitialModeChannelSettingsFile || mode == "" {
		return false
	}
	_, ok := d.ModeValues[mode]
	return ok
}

// SettingsValue returns the agent-specific settings value for a session mode.
func (d InitialModeDelivery) SettingsValue(mode string) (string, bool) {
	value, ok := d.ModeValues[mode]
	return value, ok
}

// claudeInitialModeDelivery declares Claude's start-mode channel.
//
// The bundled ACP bridge computes its initial permission mode from
// permissions.defaultMode in the settings it resolves, and overrides any
// permissionMode supplied in the session/new request, so the settings file is
// the only channel that reaches the launched process. Project and local
// settings are additionally filtered for escalating values, which leaves the
// user-scope directory named by CLAUDE_CONFIG_DIR.
func claudeInitialModeDelivery() InitialModeDelivery {
	return InitialModeDelivery{
		Channel:                  InitialModeChannelSettingsFile,
		ConfigDirEnvVar:          "CLAUDE_CONFIG_DIR",
		DefaultConfigDirTemplate: "{home}/.claude",
		SettingsFileName:         "settings.json",
		ModeKeyPath:              []string{"permissions", "defaultMode"},
		ModeValues: map[string]string{
			"default":           "default",
			"acceptEdits":       "acceptEdits",
			"plan":              "plan",
			"bypassPermissions": "bypassPermissions",
		},
		SandboxEnv: map[string]string{"IS_SANDBOX": "1"},
	}
}
