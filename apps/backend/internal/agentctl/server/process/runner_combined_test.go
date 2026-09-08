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

func TestManagerCommandEnvironmentReturnsTaskOverrides(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{
		WorkDir:   t.TempDir(),
		SessionID: "session-1",
		AgentEnv:  []string{"PATH=/task/bin", "GOBIN=/task/gobin"},
	}, newTestLogger(t))

	env, err := mgr.CommandEnvironment()
	if err != nil {
		t.Fatalf("CommandEnvironment() error = %v", err)
	}
	if env["PATH"] != "/task/bin" || env["GOBIN"] != "/task/gobin" {
		t.Fatalf("CommandEnvironment() = %#v, want task PATH and GOBIN", env)
	}
}

func TestManagerCombinedOutputCapturesFastExitStdoutAndStderr(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), SessionID: "session-1"}, newTestLogger(t))
	t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })
	command, env := fixtureExec("write-both combined-stdout combined-stderr")

	output, err := mgr.CombinedOutput(context.Background(), tools.CommandSpec{
		Path: command[0],
		Args: command[1:],
		Env:  env,
	})
	if err != nil {
		t.Fatalf("CombinedOutput() error = %v", err)
	}
	if string(output) != "combined-stdoutcombined-stderr" {
		t.Fatalf("CombinedOutput() output = %q, want both streams", output)
	}
}

func TestManagerOutputCapturesOnlyStdout(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), SessionID: "session-1"}, newTestLogger(t))
	t.Cleanup(func() { _ = mgr.StopForTeardown(context.Background()) })
	command, env := fixtureExec("write-both stdout-only stderr-only")

	output, err := mgr.Output(context.Background(), tools.CommandSpec{
		Path: command[0],
		Args: command[1:],
		Env:  env,
	})
	if err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if string(output) != "stdout-only" {
		t.Fatalf("Output() = %q, want stdout only", output)
	}
}
