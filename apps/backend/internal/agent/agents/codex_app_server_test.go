package agents

import (
	"slices"
	"testing"

	"github.com/kandev/kandev/pkg/agent"
)

func TestCodexAppServerRuntimeIdentity(t *testing.T) {
	disabled := NewCodexAppServer(false)
	if disabled.ID() != "codex-app-server" || disabled.DisplayName() != "Codex app server" {
		t.Fatalf("native identity = (%q, %q)", disabled.ID(), disabled.DisplayName())
	}
	if disabled.Enabled() {
		t.Fatal("Codex app-server must be disabled when its feature flag is off")
	}
	if !disabled.PreserveStoredProfilesWhenDisabled() {
		t.Fatal("disabled native profiles must remain readable")
	}
	if inference, ok := any(disabled).(InferenceAgent); !ok || !inference.InferenceConfig().Supported || inference.InferenceConfig().Protocol != agent.ProtocolCodexAppServer {
		t.Fatal("native Codex must expose native one-shot inference")
	}
	if _, ok := any(disabled).(OpenAICompatibleProviderAgent); ok {
		t.Fatal("native Codex must not claim OpenAI-compatible gateway support")
	}

	runtime := disabled.Runtime()
	if runtime.Protocol != agent.ProtocolCodexAppServer {
		t.Fatalf("runtime protocol = %q", runtime.Protocol)
	}
	if !runtime.SessionConfig.NativeSessionResume {
		t.Fatal("native session resume is not enabled")
	}
	want := []string{"npx", "--yes", "--prefer-offline", "--prefix", "~/.kandev/managed-npm-runtime", "@openai/codex@0.154.0", "app-server"}
	if got := runtime.Cmd.Args(); !slices.Equal(got, want) {
		t.Fatalf("runtime command = %#v, want %#v", got, want)
	}
	if got := disabled.BuildCommand(CommandOptions{}).Args(); !slices.Equal(got, want) {
		t.Fatalf("agent command = %#v, want %#v", got, want)
	}
	if got := NewCodexAppServer(true).Enabled(); !got {
		t.Fatal("enabled native agent ignored the feature flag")
	}
}
