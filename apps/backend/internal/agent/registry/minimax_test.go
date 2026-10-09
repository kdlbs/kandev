package registry

import (
	"context"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/pkg/agent"
)

func miniMaxAgent(t *testing.T) agents.Agent {
	t.Helper()
	reg := NewRegistry(newTestLogger())
	reg.LoadDefaults()
	ag, ok := reg.Get("minimax-acp")
	if !ok {
		t.Fatal("native MiniMax is missing from the built-in registry")
	}
	return ag
}

func TestMiniMaxNativeRuntime(t *testing.T) {
	ag := miniMaxAgent(t)
	if ag.DisplayName() != "MiniMax" || !ag.Enabled() {
		t.Fatalf("identity: %s enabled=%v", ag.DisplayName(), ag.Enabled())
	}
	rt := ag.Runtime()
	if rt.Protocol != agent.ProtocolACP || !slices.Equal(rt.Cmd.Args(), []string{"mcode", "acp"}) {
		t.Fatalf("runtime: %+v", rt)
	}
	if got := ag.BuildCommand(agents.CommandOptions{Model: "m:minimax:MiniMax-M3:v:thinking", AutoApprove: true}).Args(); !slices.Equal(got, []string{"mcode", "acp"}) {
		t.Fatalf("ACP command: %v", got)
	}
	inf, ok := ag.(agents.InferenceAgent)
	if !ok || !slices.Equal(inf.InferenceConfig().Command.Args(), []string{"mcode", "acp"}) {
		t.Fatal("native inference unavailable")
	}
	if !rt.SessionConfig.NativeSessionResume || rt.SessionConfig.CanRecover == nil || !*rt.SessionConfig.CanRecover {
		t.Fatal("native resume not configured")
	}
	if rt.SessionConfig.SessionDirTemplate != "{home}/.minimax" || rt.SessionConfig.SessionDirTarget != "/root/.minimax" {
		t.Fatalf("session dir: %+v", rt.SessionConfig)
	}
	if rt.ProjectMCPStrategy != nil || len(ag.PermissionSettings()) != 0 || ag.RemoteAuth() != nil {
		t.Fatal("unexpected MCP overlay, permission bypass or credential copying")
	}
	for _, key := range []string{"MINIMAX_DATA_DIR", "MAVIS_DATA_DIR", "__MAVIS_RUNTIME_DATA_DIR", "__MAVIS_RUNTIME_PROFILE"} {
		if !slices.Contains(rt.StripEnv, key) {
			t.Errorf("missing isolation for %s", key)
		}
	}
	for _, variant := range []agents.LogoVariant{agents.LogoLight, agents.LogoDark} {
		if len(ag.Logo(variant)) == 0 {
			t.Error("missing MiniMax logo")
		}
	}
	login, ok := ag.(agents.LoginAgent)
	if !ok || login.LoginCommand() == nil {
		t.Fatal("native login unavailable")
	}
	if ag.InstallScript() == "" {
		t.Fatal("official install recipe unavailable")
	}
}

func TestMiniMaxDiscoveryDoesNotInferInstallFromNpm(t *testing.T) {
	ag := miniMaxAgent(t)
	t.Setenv("PATH", t.TempDir())
	result, err := ag.IsInstalled(context.Background())
	if err != nil || result.Available {
		t.Fatalf("missing native binary: result=%+v err=%v", result, err)
	}
}

func TestMiniMaxPassthroughPreservesModelAndResume(t *testing.T) {
	ag, ok := miniMaxAgent(t).(agents.PassthroughAgent)
	if !ok {
		t.Fatal("passthrough unavailable")
	}
	cases := []struct{ model, want string }{
		{"m:minimax:MiniMax-M3:v:thinking", "minimax/MiniMax-M3#thinking"},
		{"m:minimax:MiniMax-M2.7:u", "minimax/MiniMax-M2.7"},
		{"m:custom%3Aprovider:model%20id:v:variant%2B", "custom:provider/model id#variant+"},
		{"minimax/MiniMax-M3", "minimax/MiniMax-M3"},
		{"m:minimax:model:v:", "minimax/model#none-thinking"},
		{"m:minimax:model:u:extra", "m:minimax:model:u:extra"},
		{"m:%ZZ:model:u", "m:%ZZ:model:u"},
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			got := ag.BuildPassthroughCommand(agents.PassthroughOptions{Model: tc.model, SessionID: "native-session"}).Args()
			want := []string{"mcode", "--model", tc.want, "--session", "native-session"}
			if !slices.Equal(got, want) {
				t.Fatalf("command = %v, want %v", got, want)
			}
		})
	}
	if got := ag.BuildPassthroughCommand(agents.PassthroughOptions{Resume: true}).Args(); !slices.Equal(got, []string{"mcode", "--continue"}) {
		t.Fatalf("continue: %v", got)
	}
	if got := ag.BuildPassthroughCommand(agents.PassthroughOptions{Prompt: "inspect the project"}).Args(); !slices.Equal(got, []string{"mcode", "inspect the project"}) {
		t.Fatalf("prompt: %v", got)
	}
}
