package lifecycle

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

func newCopilotPassthroughResetFixture(t *testing.T, version, executorType string) (*Manager, *AgentExecution, *recordingPassthroughRunner, *copilotPluginRunner, string, string) {
	t.Helper()
	root, configDir, pluginRunner := copilotPluginFixture(t)
	profile := &AgentProfileInfo{
		ProfileID: "profile-1", ProfileName: "passthrough-reset",
		AgentName: agents.CopilotACPAgentID, CLIPassthrough: true,
		CopilotSelectedPlugins: []string{"installed:copilot-plugins/spark"},
		EnvVars: []models.ProfileEnvVar{
			{Key: "COPILOT_HOME", Value: configDir},
			{Key: "PATH", Value: os.Getenv("PATH")},
		},
	}
	manager, execution, _ := newPassthroughMCPTestManager(t, agents.CopilotACPAgentID)
	manager.profileResolver = &mockPassthroughProfileResolver{
		agentName: agents.CopilotACPAgentID, cliPassthrough: true, profile: profile,
	}
	manager.managedRuntimeSelections = &copilotSelectionReader{version: version}
	manager.SetCopilotPluginRunner(pluginRunner)
	execution.AgentID = agents.CopilotACPAgentID
	execution.AgentProfileID = profile.ProfileID
	execution.ExecutionProfileID = profile.ProfileID
	execution.ExecutorType = executorType
	execution.CopilotSelectedPlugins = append([]string{}, profile.CopilotSelectedPlugins...)
	execution.PassthroughProcessID = "pty-old"
	execution.PassthroughStartedAt = time.Now()
	execution.setRuntimeEnvironment(map[string]string{
		"COPILOT_HOME": configDir,
		"PATH":         os.Getenv("PATH"),
	})
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}
	pty := &recordingPassthroughRunner{}
	manager.passthroughRunnerOverride = pty
	return manager, execution, pty, pluginRunner, root, configDir
}

func TestCopilotPluginSupportedPassthroughResume(t *testing.T) {
	manager, execution, pty, runner, root, configDir := newCopilotPassthroughResetFixture(t, "1.0.104", "local")
	if err := manager.ResumePassthroughSession(context.Background(), execution.SessionID); err != nil {
		t.Fatalf("ResumePassthroughSession: %v", err)
	}
	if pty.startCalls != 1 {
		t.Fatalf("resumed passthrough starts = %d, want one", pty.startCalls)
	}
	if !containsSequence(pty.startReq.Command, "--continue") ||
		!containsSequence(pty.startReq.Command, "--plugin-dir", canonicalCopilotPluginRootForTest(t, root)) ||
		countArg(pty.startReq.Command, "--plugin-dir") != 1 {
		t.Fatalf("resumed passthrough argv = %#v, want resume plus one selected mount", pty.startReq.Command)
	}
	if version, ok := copilotplugins.RuntimeVersionFromCommand(pty.startReq.Command); !ok || version != "1.0.104" {
		t.Fatalf("resumed passthrough command = %#v, want selected runtime 1.0.104", pty.startReq.Command)
	}
	if pty.startReq.WorkingDir != execution.WorkspacePath ||
		pty.startReq.Env["COPILOT_HOME"] != configDir {
		t.Fatalf("resumed passthrough context = cwd %q env %#v, want cwd %q and COPILOT_HOME %q",
			pty.startReq.WorkingDir, pty.startReq.Env, execution.WorkspacePath, configDir)
	}
	if len(runner.calls) != 1 ||
		!containsSequence(runner.calls[0], "@github/copilot@1.0.104", "plugin", "list") ||
		runner.dirs[0] != execution.WorkspacePath {
		t.Fatalf("resume inventory = %#v dirs=%#v, want selected runtime in workspace", runner.calls, runner.dirs)
	}
}

func TestCopilotPluginSupportedPassthroughResumeFallback(t *testing.T) {
	manager, execution, pty, runner, root, configDir := newCopilotPassthroughResetFixture(t, "1.0.104", "local")
	pty.activeWebSocket = true
	execution.passthroughLaunchUsedResume = true
	manager.attemptResumeFallback(execution, pty, execution.SessionID, 1, 400*time.Millisecond)

	if pty.startCalls != 1 {
		t.Fatalf("resume fallback starts = %d, want one", pty.startCalls)
	}
	if containsSequence(pty.startReq.Command, "--continue") ||
		!containsSequence(pty.startReq.Command, "--plugin-dir", canonicalCopilotPluginRootForTest(t, root)) ||
		countArg(pty.startReq.Command, "--plugin-dir") != 1 {
		t.Fatalf("fallback argv = %#v, want fresh command plus one selected mount", pty.startReq.Command)
	}
	if version, ok := copilotplugins.RuntimeVersionFromCommand(pty.startReq.Command); !ok || version != "1.0.104" {
		t.Fatalf("fallback command = %#v, want selected runtime 1.0.104", pty.startReq.Command)
	}
	if pty.startReq.WorkingDir != execution.WorkspacePath ||
		pty.startReq.Env["COPILOT_HOME"] != configDir {
		t.Fatalf("fallback context = cwd %q env %#v, want cwd %q and COPILOT_HOME %q",
			pty.startReq.WorkingDir, pty.startReq.Env, execution.WorkspacePath, configDir)
	}
	if len(runner.calls) != 1 ||
		!containsSequence(runner.calls[0], "@github/copilot@1.0.104", "plugin", "list") ||
		runner.dirs[0] != execution.WorkspacePath {
		t.Fatalf("fallback inventory = %#v dirs=%#v, want selected runtime in workspace", runner.calls, runner.dirs)
	}
}

func TestCopilotPluginSupportedPassthroughRetainedProcessSkipsPluginWork(t *testing.T) {
	manager, execution, pty, runner, _, _ := newCopilotPassthroughResetFixture(t, "1.0.104", "local")
	pty.readyOrPending = true
	oldProcessID := execution.PassthroughProcessID
	if err := manager.startPassthroughSession(context.Background(), execution, nil); err != nil {
		t.Fatalf("retained passthrough start: %v", err)
	}
	if pty.startCalls != 0 || pty.stopCalls != 0 || execution.PassthroughProcessID != oldProcessID {
		t.Fatalf("retained process changed: starts=%d stops=%d process=%q",
			pty.startCalls, pty.stopCalls, execution.PassthroughProcessID)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("retained process ran %d native inventories, want none", len(runner.calls))
	}
}

func TestCopilotPluginPassthroughResetPreflightPreservesProcess(t *testing.T) {
	t.Run("unsupported runtime does not stop local process", func(t *testing.T) {
		manager, execution, pty, runner, _, _ := newCopilotPassthroughResetFixture(t, "1.0.200", "local")
		err := manager.restartPassthroughProcess(context.Background(), execution)
		var pluginErr *copilotplugins.Error
		if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonRuntimeUnsupported {
			t.Fatalf("reset error = %#v, want typed runtime-unsupported error", err)
		}
		if pty.stopCalls != 0 || pty.startCalls != 0 || execution.PassthroughProcessID != "pty-old" {
			t.Fatalf("rejected reset changed process: stops=%d starts=%d identity=%q",
				pty.stopCalls, pty.startCalls, execution.PassthroughProcessID)
		}
		if len(runner.calls) != 0 {
			t.Fatalf("unsupported runtime ran %d inventory commands", len(runner.calls))
		}
	})

	t.Run("supported local reset preflights and replaces once", func(t *testing.T) {
		manager, execution, pty, runner, root, configDir := newCopilotPassthroughResetFixture(t, "1.0.104", "local")
		var inventoryAfterStop bool
		runner.onRun = func() {
			inventoryAfterStop = inventoryAfterStop || pty.stopCalls > 0
		}
		if err := manager.restartPassthroughProcess(context.Background(), execution); err != nil {
			t.Fatalf("restartPassthroughProcess: %v", err)
		}
		if pty.stopCalls != 1 || pty.startCalls != 1 {
			t.Fatalf("reset stop/start counts = %d/%d, want one each", pty.stopCalls, pty.startCalls)
		}
		if inventoryAfterStop || len(runner.calls) != 1 {
			t.Fatalf("inventory after stop=%t calls=%d, want one preflight before stop", inventoryAfterStop, len(runner.calls))
		}
		if !containsSequence(pty.startReq.Command, "--plugin-dir", canonicalCopilotPluginRootForTest(t, root)) ||
			countArg(pty.startReq.Command, "--plugin-dir") != 1 {
			t.Fatalf("reset command = %#v, want one selected plugin mount", pty.startReq.Command)
		}
		if pty.startReq.WorkingDir != execution.WorkspacePath {
			t.Fatalf("reset working directory = %q, want %q", pty.startReq.WorkingDir, execution.WorkspacePath)
		}
		if pty.startReq.Env["COPILOT_HOME"] != configDir ||
			pty.startReq.Env["COPILOT_HOME"] != runner.envs[0]["COPILOT_HOME"] {
			t.Fatalf("reset environment COPILOT_HOME=%q inventory=%q, want %q",
				pty.startReq.Env["COPILOT_HOME"], runner.envs[0]["COPILOT_HOME"], configDir)
		}
		if !containsSequence(runner.calls[0], "@github/copilot@1.0.104", "plugin", "list") {
			t.Fatalf("reset inventory command = %#v", runner.calls[0])
		}
	})

	t.Run("isolated SSH selection is rejected before remote stop", func(t *testing.T) {
		manager, execution, pty, runner, _, _ := newCopilotPassthroughResetFixture(t, "1.0.104", "ssh")
		err := manager.restartPassthroughProcess(context.Background(), execution)
		var pluginErr *copilotplugins.Error
		if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonUnsupportedLaunch {
			t.Fatalf("SSH reset error = %#v, want typed unsupported-launch error", err)
		}
		if pty.stopCalls != 0 || pty.startCalls != 0 || execution.PassthroughProcessID != "pty-old" {
			t.Fatalf("rejected SSH reset changed process: stops=%d starts=%d identity=%q",
				pty.stopCalls, pty.startCalls, execution.PassthroughProcessID)
		}
		if len(runner.calls) != 0 {
			t.Fatalf("isolated SSH selection ran %d host inventory commands", len(runner.calls))
		}
	})
}
