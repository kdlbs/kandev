package process

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	tools "github.com/kandev/kandev/internal/tools/installer"
)

func TestManagerCombinedOutputCapturesFailureOutput(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), SessionID: "session-1"}, newTestLogger(t))
	t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })
	command, env := fixtureExec("unknown-command")

	output, err := mgr.CombinedOutput(context.Background(), tools.CommandSpec{
		Path: command[0],
		Args: command[1:],
		Env:  env,
	})
	if err == nil {
		t.Fatal("CombinedOutput() error = nil, want fixture failure")
	}
	if !strings.Contains(string(output), "unknown command") {
		t.Fatalf("CombinedOutput() output = %q, want fixture stderr", output)
	}
}
