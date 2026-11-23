package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentruntime"
)

func TestSessionInferenceUsesSelectedManagedRuntimeVersion(t *testing.T) {
	copilot := agents.NewCopilotACP()
	harness := newInferenceHarness(t, nil)
	harness.manager.managedRuntimeSelections = &copilotSelectionReader{version: "1.0.104"}

	if _, err := harness.manager.ExecuteInferencePrompt(t.Context(), "session-1", copilot.ID(), "", "hi"); err != nil {
		t.Fatalf("ExecuteInferencePrompt: %v", err)
	}
	recorded := harness.recorded()
	if len(recorded) != 1 {
		t.Fatalf("agentctl calls = %d, want 1", len(recorded))
	}
	version, ok := copilotplugins.RuntimeVersionFromCommand(recorded[0].InferenceConfig.Command)
	if !ok || version != "1.0.104" {
		t.Fatalf("session inference command = %v, want the selected @github/copilot@1.0.104", recorded[0].InferenceConfig.Command)
	}
}

type openCodeV2SelectionReader struct{ spec agents.ManagedNPMRuntimeSpec }

func (r openCodeV2SelectionReader) Get(context.Context, string, string) (managedruntime.Selection, bool, error) {
	return managedruntime.Selection{}, false, nil
}

func (r openCodeV2SelectionReader) GetOpenCodeSelection(context.Context) (managedruntime.OpenCodeSelection, bool, error) {
	return managedruntime.OpenCodeSelection{
		SchemaVersion:         1,
		Family:                managedruntime.OpenCodeFamilyV2,
		Source:                managedruntime.OpenCodeSourceManaged,
		Package:               r.spec.Package,
		AppliedDefaultVersion: r.spec.DefaultVersionOrPinned(),
		Revision:              1,
	}, true, nil
}

func TestSessionInferenceUsesSelectedOpenCodeFamily(t *testing.T) {
	openCode := agents.NewOpenCodeACP()
	v2, err := openCode.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV2)
	if err != nil {
		t.Fatal(err)
	}
	harness := newInferenceHarness(t, nil)
	harness.manager.managedRuntimeSelections = openCodeV2SelectionReader{spec: v2}

	if _, err := harness.manager.ExecuteInferencePrompt(t.Context(), "session-1", openCode.ID(), "", "hi"); err != nil {
		t.Fatalf("ExecuteInferencePrompt: %v", err)
	}
	got := harness.recorded()[0].InferenceConfig.Command
	if !slices.Contains(got, v2.PackageSpec(v2.DefaultVersionOrPinned())) {
		t.Fatalf("session inference command = %v, want the selected v2 package %s", got, v2.PackageSpec(v2.DefaultVersionOrPinned()))
	}
}

func TestSessionInferenceProfileUsesSelectedManagedRuntimeVersion(t *testing.T) {
	copilot := agents.NewCopilotACP()
	harness := newInferenceHarness(t, &stubProfileResolver{profile: &AgentProfileInfo{
		ProfileID: "profile-1",
		AgentID:   copilot.ID(),
		AgentName: copilot.ID(),
		Model:     "gpt-4.1",
	}})
	harness.manager.managedRuntimeSelections = &copilotSelectionReader{version: "1.0.104"}

	if _, err := harness.manager.ExecuteInferenceProfilePrompt(t.Context(), "session-1", "profile-1", "hi"); err != nil {
		t.Fatalf("ExecuteInferenceProfilePrompt: %v", err)
	}
	recorded := harness.recorded()
	if len(recorded) != 1 {
		t.Fatalf("agentctl calls = %d, want 1", len(recorded))
	}
	version, ok := copilotplugins.RuntimeVersionFromCommand(recorded[0].InferenceConfig.Command)
	if !ok || version != "1.0.104" {
		t.Fatalf("profile inference command = %v, want the selected @github/copilot@1.0.104", recorded[0].InferenceConfig.Command)
	}
}

type nativeOpenCodeSelectionReader struct{ spec agents.ManagedNPMRuntimeSpec }

func (r nativeOpenCodeSelectionReader) Get(context.Context, string, string) (managedruntime.Selection, bool, error) {
	return managedruntime.Selection{}, false, nil
}

func (r nativeOpenCodeSelectionReader) GetOpenCodeSelection(context.Context) (managedruntime.OpenCodeSelection, bool, error) {
	return managedruntime.OpenCodeSelection{
		SchemaVersion:         1,
		Family:                managedruntime.OpenCodeFamilyV1,
		Source:                managedruntime.OpenCodeSourceNative,
		Package:               r.spec.Package,
		AppliedDefaultVersion: r.spec.DefaultVersionOrPinned(),
		Revision:              1,
	}, true, nil
}

func TestSessionInferenceUsesManagedOpenCodeOnRemoteRuntime(t *testing.T) {
	openCode := agents.NewOpenCodeACP()
	harness := newInferenceHarness(t, nil)
	harness.manager.managedRuntimeSelections = nativeOpenCodeSelectionReader{spec: openCode.ManagedNPMRuntime()}
	execution, ok := harness.manager.executionStore.GetBySessionID("session-1")
	if !ok {
		t.Fatal("session execution is unavailable")
	}
	execution.AgentID = "claude-acp"
	execution.RuntimeName = agentruntime.RuntimeSSH
	execution.setMetadataValue(MetadataKeyNativeBinary, openCode.NativeBinaryName())

	if _, err := harness.manager.ExecuteInferencePrompt(t.Context(), "session-1", openCode.ID(), "", "hi"); err != nil {
		t.Fatalf("ExecuteInferencePrompt: %v", err)
	}
	command := harness.recorded()[0].InferenceConfig.Command
	if command[0] != "npx" || !slices.Contains(command, openCode.ManagedNPMRuntime().PackageSpec("")) {
		t.Fatalf("remote session inference command = %v, want the selected managed runtime", command)
	}
}

func TestSessionInferenceUsesNativeOpenCodeOnStandaloneRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake OpenCode executable uses a POSIX shell script")
	}

	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte("#!/bin/sh\nprintf 'opencode 1.18.32\\n'\n"), 0o755); err != nil {
		t.Fatalf("write fake OpenCode binary: %v", err)
	}
	t.Setenv("PATH", binDir)

	openCode := agents.NewOpenCodeACP()
	harness := newInferenceHarness(t, nil)
	harness.manager.managedRuntimeSelections = nativeOpenCodeSelectionReader{spec: openCode.ManagedNPMRuntime()}
	execution, ok := harness.manager.executionStore.GetBySessionID("session-1")
	if !ok {
		t.Fatal("session execution is unavailable")
	}
	execution.RuntimeName = agentruntime.RuntimeStandalone

	if _, err := harness.manager.ExecuteInferencePrompt(t.Context(), "session-1", openCode.ID(), "", "hi"); err != nil {
		t.Fatalf("ExecuteInferencePrompt: %v", err)
	}
	command := harness.recorded()[0].InferenceConfig.Command
	if command[0] != openCode.NativeBinaryName() || !slices.Contains(command, "acp") {
		t.Fatalf("standalone session inference command = %v, want native OpenCode ACP command", command)
	}
}

type failingOpenCodeSelectionReader struct{ err error }

func (r failingOpenCodeSelectionReader) Get(context.Context, string, string) (managedruntime.Selection, bool, error) {
	return managedruntime.Selection{}, false, nil
}

func (r failingOpenCodeSelectionReader) GetOpenCodeSelection(context.Context) (managedruntime.OpenCodeSelection, bool, error) {
	return managedruntime.OpenCodeSelection{}, false, r.err
}

func TestSessionInferenceReturnsManagedRuntimeSelectionError(t *testing.T) {
	wantErr := errors.New("selection store unavailable")
	openCode := agents.NewOpenCodeACP()
	harness := newInferenceHarness(t, nil)
	harness.manager.managedRuntimeSelections = failingOpenCodeSelectionReader{err: wantErr}

	_, err := harness.manager.ExecuteInferencePrompt(t.Context(), "session-1", openCode.ID(), "", "hi")
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExecuteInferencePrompt error = %v, want %v", err, wantErr)
	}
	if recorded := harness.recorded(); len(recorded) != 0 {
		t.Fatalf("agentctl calls = %d, want 0", len(recorded))
	}
}

type emptyCommandOpenCode struct{ *agents.OpenCodeACP }

func (a *emptyCommandOpenCode) BuildCommand(agents.CommandOptions) agents.Command {
	return agents.Command{}
}

func TestSessionInferenceRejectsEmptyBuiltCommand(t *testing.T) {
	openCode := &emptyCommandOpenCode{OpenCodeACP: agents.NewOpenCodeACP()}
	harness := newInferenceHarness(t, nil)
	if err := harness.manager.registry.Replace(openCode); err != nil {
		t.Fatalf("replace OpenCode agent: %v", err)
	}

	_, err := harness.manager.ExecuteInferencePrompt(t.Context(), "session-1", openCode.ID(), "", "hi")
	if err == nil || !strings.Contains(err.Error(), "agent command cannot be empty") {
		t.Fatalf("ExecuteInferencePrompt error = %v, want an empty-command validation error", err)
	}
	if recorded := harness.recorded(); len(recorded) != 0 {
		t.Fatalf("agentctl calls = %d, want 0", len(recorded))
	}
}

func TestSessionInferenceHoldsOpenCodeAdmissionUntilAgentctlReturns(t *testing.T) {
	openCode := agents.NewOpenCodeACP()
	harness := newInferenceHarness(t, nil)
	v2, err := openCode.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV2)
	if err != nil {
		t.Fatal(err)
	}
	harness.manager.managedRuntimeSelections = openCodeV2SelectionReader{spec: v2}
	execution, ok := harness.manager.executionStore.GetBySessionID("session-1")
	if !ok {
		t.Fatal("session execution is unavailable")
	}
	execution.AgentID = "claude-acp"
	harness.promptStarted = make(chan struct{})
	harness.releasePromptAgent = make(chan struct{})
	var releasePrompt sync.Once
	defer releasePrompt.Do(func() { close(harness.releasePromptAgent) })

	promptDone := make(chan error, 1)
	go func() {
		_, promptErr := harness.manager.ExecuteInferencePrompt(t.Context(), "session-1", openCode.ID(), "", "hi")
		promptDone <- promptErr
	}()
	<-harness.promptStarted

	lockWasFree := harness.manager.openCodeAdmission.TryLock()
	if lockWasFree {
		harness.manager.openCodeAdmission.Unlock()
	}
	releasePrompt.Do(func() { close(harness.releasePromptAgent) })
	if err := <-promptDone; err != nil {
		t.Fatalf("ExecuteInferencePrompt: %v", err)
	}
	if lockWasFree {
		t.Fatal("OpenCode migration admission was available while its session utility prompt was active")
	}

	_, releaseMigration, err := harness.manager.AcquireOpenCodeMigration(t.Context())
	if err != nil {
		t.Fatalf("AcquireOpenCodeMigration after prompt returned: %v", err)
	}
	releaseMigration()
}
