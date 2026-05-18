package process

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type stubAgentAdapter struct{}

func (stubAgentAdapter) PrepareEnvironment() (map[string]string, error) { return nil, nil }
func (stubAgentAdapter) PrepareCommandArgs() []string                   { return nil }
func (stubAgentAdapter) Connect(io.Writer, io.Reader) error             { return nil }
func (stubAgentAdapter) Initialize(context.Context) error               { return nil }
func (stubAgentAdapter) GetAgentInfo() *adapter.AgentInfo               { return nil }
func (stubAgentAdapter) NewSession(context.Context, []types.McpServer) (string, error) {
	return "", nil
}
func (stubAgentAdapter) LoadSession(context.Context, string, []types.McpServer) error {
	return nil
}
func (stubAgentAdapter) Prompt(context.Context, string, []v1.MessageAttachment) error {
	return nil
}
func (stubAgentAdapter) Cancel(context.Context) error                   { return nil }
func (stubAgentAdapter) Updates() <-chan adapter.AgentEvent             { return nil }
func (stubAgentAdapter) GetSessionID() string                           { return "" }
func (stubAgentAdapter) GetOperationID() string                         { return "" }
func (stubAgentAdapter) SetPermissionHandler(adapter.PermissionHandler) {}
func (stubAgentAdapter) Close() error                                   { return nil }
func (stubAgentAdapter) RequiresProcessKill() bool                      { return false }

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if len(item) > len(prefix) && item[:len(prefix)] == prefix {
			return item[len(prefix):]
		}
	}
	return ""
}

func TestManager_BuildFinalCommandInjectsIsolatedTempDir(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		SessionID: "session-123",
		AgentArgs: []string{"echo"},
		AgentEnv:  []string{"PATH=/usr/bin"},
	}, newTestLogger(t))
	mgr.adapter = stubAgentAdapter{}

	if err := mgr.buildFinalCommand(); err != nil {
		t.Fatalf("buildFinalCommand() error = %v", err)
	}

	want := filepath.Join(os.TempDir(), "kandev-agent", "session-123")
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		if got := envValue(mgr.cmd.Env, key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("expected temp dir %q to exist, stat=%v err=%v", want, info, err)
	}
}
