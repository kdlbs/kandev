package mcpconfig

// Strategy keys are the stable identifiers used to persist a passthrough MCP
// strategy choice (custom TUI agents store one on their tui_config) and to
// carry it over the settings API. Built-in agents declare their strategy in Go
// and never round-trip through a key.
//
// A custom TUI agent wraps an arbitrary command — `zsh -ic "fuelclaude --model
// opus"` is a real example — so kandev cannot infer which CLI is underneath or
// how that CLI loads MCP servers. Guessing wrong is the worst outcome
// available: the config file gets written, the CLI ignores it, and nothing
// anywhere reports an error. The user picks the key instead.
const (
	// StrategyKeyNone means the agent does not get MCP injection. It is the
	// default for custom TUI agents and the stored value for every agent
	// created before this field existed.
	StrategyKeyNone     = ""
	StrategyKeyClaude   = "claude"
	StrategyKeyCodex    = "codex"
	StrategyKeyCursor   = "cursor"
	StrategyKeyOpenCode = "opencode"
	StrategyKeyPi       = "pi"
)

// StrategyOption describes one selectable strategy for the settings UI.
type StrategyOption struct {
	Key string `json:"key"`
	// Description is the strategy's own Describe() text, so the UI explains the
	// actual mechanism ("an MCP config file passed via the --mcp-config flag")
	// rather than restating the key.
	Description string `json:"description"`
}

// strategyByKey is the single source of truth for key → implementation.
// StrategyOptions and StrategyByKey both read it, so a new strategy becomes
// selectable in the UI and resolvable at launch from one edit.
var strategyByKey = map[string]PassthroughMCPStrategy{
	StrategyKeyClaude:   ClaudeStrategy{},
	StrategyKeyCodex:    CodexStrategy{},
	StrategyKeyCursor:   CursorStrategy{},
	StrategyKeyOpenCode: OpenCodeStrategy{},
	StrategyKeyPi:       PiStrategy{},
}

// strategyKeyOrder fixes the presentation order of StrategyOptions. Map
// iteration order is random, and a dropdown that reshuffles on every request is
// unusable.
var strategyKeyOrder = []string{
	StrategyKeyClaude,
	StrategyKeyCodex,
	StrategyKeyCursor,
	StrategyKeyOpenCode,
	StrategyKeyPi,
}

// StrategyByKey resolves a persisted strategy key to its implementation.
//
// The empty key resolves to (nil, true): "this agent does no MCP injection" is
// a valid stored value, not a failure. An unknown key returns ok=false so the
// API layer can reject it up front — resolving it to nil instead would persist
// a typo that silently disables MCP forever.
func StrategyByKey(key string) (PassthroughMCPStrategy, bool) {
	if key == StrategyKeyNone {
		return nil, true
	}
	strategy, ok := strategyByKey[key]
	return strategy, ok
}

// StrategyOptions returns the selectable strategies in stable order, each with
// the mechanism description the UI shows next to it.
func StrategyOptions() []StrategyOption {
	options := make([]StrategyOption, 0, len(strategyKeyOrder))
	for _, key := range strategyKeyOrder {
		options = append(options, StrategyOption{
			Key:         key,
			Description: strategyByKey[key].Describe(),
		})
	}
	return options
}
