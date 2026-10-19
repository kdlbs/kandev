package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

// The launch command follows the backend the caller resolved, not the
// requested executor type: a request with no executor type (or one that falls
// back to another backend) still runs on the resolved standalone backend, so a
// native OpenCode selection must yield its native command rather than an empty
// one.
func TestBuildExecutionUsesResolvedStandaloneForNativeOpenCode(t *testing.T) {
	const nativeVersion = "1.18.32"
	writeNativeOpenCodeVersionFixture(t, nativeVersion)
	manager := &Manager{commandBuilder: NewCommandBuilder(), logger: newTestLogger()}
	manager.SetManagedRuntimeSelectionStore(managedRuntimeOpenCodeStore{openCodeSelection: managedruntime.OpenCodeSelection{
		SchemaVersion: 1, Family: managedruntime.OpenCodeFamilyV1, Source: managedruntime.OpenCodeSourceNative,
		Package: "opencode-ai", AppliedDefaultVersion: nativeVersion, Revision: 1,
	}})
	agent := agents.NewOpenCodeACP()
	nativeArgs, err := agents.OpenCodeACPArgsForVersion(nativeVersion)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string{"opencode"}, nativeArgs...)
	for _, executorType := range []string{"", "unknown", string(models.ExecutorTypeLocal), string(models.ExecutorTypeLocalDocker)} {
		t.Run("requested_"+executorType, func(t *testing.T) {
			execution, err := manager.buildExecutionFromInstance(context.Background(),
				&LaunchRequest{ExecutorType: executorType},
				&ExecutorCreateRequest{ExecutorType: executorType, AgentConfig: agent},
				&ExecutorInstance{InstanceID: "test-instance"}, &StandaloneExecutor{}, nil, agent, nil)
			if err != nil {
				t.Fatalf("build resolved standalone execution: %v", err)
			}
			if execution.RuntimeName != agentruntime.RuntimeStandalone || !reflect.DeepEqual(execution.AgentArgs, want) {
				t.Fatalf("runtime=%s args=%q, want standalone native v1 command %q", execution.RuntimeName, execution.AgentArgs, want)
			}
		})
	}
}

func TestPromoteWorkspaceExecutionUsesResolvedStandaloneForNativeOpenCode(t *testing.T) {
	writeNativeOpenCodeVersionFixture(t, "1.18.32")
	for _, requested := range []string{"", "unknown", string(models.ExecutorTypeLocal), string(models.ExecutorTypeLocalDocker)} {
		t.Run("requested_"+requested, func(t *testing.T) {
			m := newTestManager(t)
			m.profileResolver = nil
			m.SetManagedRuntimeSelectionStore(nativeOpenCodeAdmissionManager("1.18.32").managedRuntimeSelections)
			execution := &AgentExecution{
				ID: "workspace-execution", TaskID: "task-1", SessionID: "session-1",
				RuntimeName: agentruntime.RuntimeStandalone, WorkspacePath: t.TempDir(),
			}
			req := &LaunchRequest{
				TaskID: execution.TaskID, SessionID: execution.SessionID,
				AgentProfileID: agents.OpenCodeACPAgentID, ExecutorType: requested,
				IsPassthrough: true,
			}
			if err := m.promoteWorkspaceExecution(context.Background(), execution, req); err != nil {
				t.Fatalf("promote resolved standalone execution: %v", err)
			}
			if want := []string{"opencode", "acp", "--print-logs"}; !reflect.DeepEqual(execution.AgentArgs, want) {
				t.Fatalf("promoted command = %q, want %q", execution.AgentArgs, want)
			}
		})
	}
}

// writeNativeOpenCodeVersionFixture puts an opencode executable on PATH that
// only answers the version probe, so no agent or host CLI is started.
func writeNativeOpenCodeVersionFixture(t *testing.T, version string) {
	t.Helper()
	dir := t.TempDir()
	name, body := "opencode", "#!/bin/sh\nprintf 'opencode "+version+"\n'\n"
	if runtime.GOOS == "windows" {
		name, body = "opencode.cmd", "@echo opencode "+version+"\r\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}
