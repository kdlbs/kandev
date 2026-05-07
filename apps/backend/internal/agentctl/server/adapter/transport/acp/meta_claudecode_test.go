package acp

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
)

func TestBuildClaudeCodeMeta_Empty(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"nil":             nil,
		"empty slice":     {},
		"only orphan val": {"value-without-flag"},
		"empty flag":      {"--"},
	}
	for name, tokens := range cases {
		t.Run(name, func(t *testing.T) {
			if got := buildClaudeCodeMeta(tokens); got != nil {
				t.Errorf("expected nil meta, got %#v", got)
			}
		})
	}
}

func TestBuildClaudeCodeMeta_BareFlag(t *testing.T) {
	t.Parallel()
	got := buildClaudeCodeMeta([]string{"--debug"})
	want := claudeCodeMeta(map[string]any{"debug": ""})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestBuildClaudeCodeMeta_FlagWithValue(t *testing.T) {
	t.Parallel()
	got := buildClaudeCodeMeta([]string{"--plugin-dir", "/path/to/plugin"})
	want := claudeCodeMeta(map[string]any{"plugin-dir": "/path/to/plugin"})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestBuildClaudeCodeMeta_EqualsForm(t *testing.T) {
	t.Parallel()
	got := buildClaudeCodeMeta([]string{"--plugin-dir=/path/with=eq"})
	want := claudeCodeMeta(map[string]any{"plugin-dir": "/path/with=eq"})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestBuildClaudeCodeMeta_MixedTokens(t *testing.T) {
	t.Parallel()
	got := buildClaudeCodeMeta([]string{
		"--plugin-dir", "C:\\src\\TheOne\\plugin",
		"--debug",
		"--model=opus",
		"--add-dir", "/another/path",
	})
	want := claudeCodeMeta(map[string]any{
		"plugin-dir": "C:\\src\\TheOne\\plugin",
		"debug":      "",
		"model":      "opus",
		"add-dir":    "/another/path",
	})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestBuildClaudeCodeMeta_TwoBareFlagsInARow(t *testing.T) {
	t.Parallel()
	// `--foo --bar` — both bare; the second --bar starts with -- so the
	// look-ahead for --foo must not consume it as a value.
	got := buildClaudeCodeMeta([]string{"--foo", "--bar"})
	want := claudeCodeMeta(map[string]any{"foo": "", "bar": ""})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestBuildClaudeCodeMeta_OrphanValueBeforeFlagIgnored(t *testing.T) {
	t.Parallel()
	// Orphan tokens (not preceded by a flag) are silently skipped — they
	// would have been consumed if a preceding flag existed.
	got := buildClaudeCodeMeta([]string{"orphan", "--key", "value"})
	want := claudeCodeMeta(map[string]any{"key": "value"})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestBuildSessionMeta_DispatchClaudeACP(t *testing.T) {
	t.Parallel()
	a := &Adapter{
		agentID: "claude-acp",
		cfg:     mustSharedConfig([]string{"--plugin-dir", "/p"}),
	}
	got := a.buildSessionMeta()
	want := claudeCodeMeta(map[string]any{"plugin-dir": "/p"})
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestBuildSessionMeta_DispatchUnknownAgentReturnsNil(t *testing.T) {
	t.Parallel()
	a := &Adapter{
		agentID: "auggie",
		cfg:     mustSharedConfig([]string{"--plugin-dir", "/p"}),
	}
	if got := a.buildSessionMeta(); got != nil {
		t.Errorf("expected nil meta for non-claude agent, got %#v", got)
	}
}

func TestBuildSessionMeta_NoTokensReturnsNil(t *testing.T) {
	t.Parallel()
	a := &Adapter{
		agentID: "claude-acp",
		cfg:     mustSharedConfig(nil),
	}
	if got := a.buildSessionMeta(); got != nil {
		t.Errorf("expected nil meta when no CLIFlagTokens, got %#v", got)
	}
}

// claudeCodeMeta wraps an extraArgs map in the nested structure that the
// bridge expects: _meta.claudeCode.options.extraArgs.
func claudeCodeMeta(extraArgs map[string]any) map[string]any {
	return map[string]any{
		"claudeCode": map[string]any{
			"options": map[string]any{
				"extraArgs": extraArgs,
			},
		},
	}
}

// mustSharedConfig builds a minimal shared.Config carrying the given CLI flag
// tokens. Used in dispatcher tests where only CLIFlagTokens matter.
func mustSharedConfig(tokens []string) *shared.Config {
	return &shared.Config{CLIFlagTokens: tokens}
}
