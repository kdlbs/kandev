package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// denyDockerCapabilities records registration admission and refuses the
// Docker selection, proving admission runs before preparation effects.
type denyDockerCapabilities struct {
	validateCalls []string
}

func (c *denyDockerCapabilities) RequiresCloneURL(string) bool          { return false }
func (c *denyDockerCapabilities) ShouldApplyPreferredShell(string) bool { return false }

func (c *denyDockerCapabilities) ValidateExecutorRuntime(executorType string) error {
	c.validateCalls = append(c.validateCalls, executorType)
	if executorType == string(models.ExecutorTypeLocalDocker) {
		return lifecycle.ErrExecutorRuntimeUnavailable
	}
	return nil
}

func (c *denyDockerCapabilities) ValidateExecutionRuntime(context.Context, string, string, string) error {
	return nil
}

// TestExplicitRuntimeAdmissionBeforeEffects proves an unavailable required
// runtime is refused before session persistence, environment creation, or
// credential preparation for both repository-free and repository-backed
// preparation.
func TestExplicitRuntimeAdmissionBeforeEffects(t *testing.T) {
	for _, repoBacked := range []bool{false, true} {
		name := "repository_free"
		if repoBacked {
			name = "repository_backed"
		}
		t.Run(name, func(t *testing.T) {
			repo := newMockRepository()
			agentManager := &mockAgentManager{}
			exec := newTestExecutor(t, agentManager, repo)
			capabilities := &denyDockerCapabilities{}
			exec.SetCapabilities(capabilities)

			task := &v1.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Task"}
			if repoBacked {
				repo.taskRepositories["task-repo-1"] = &models.TaskRepository{
					ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1",
					BaseBranch: "main", Position: 0,
				}
			}

			_, err := exec.PrepareSession(
				context.Background(), task, "profile-1", models.ExecutorIDLocalDocker, "", "",
			)
			if !errors.Is(err, lifecycle.ErrExecutorRuntimeUnavailable) {
				t.Fatalf("PrepareSession error = %v, want runtime unavailable", err)
			}
			if len(capabilities.validateCalls) != 1 ||
				capabilities.validateCalls[0] != string(models.ExecutorTypeLocalDocker) {
				t.Fatalf("admission calls = %v", capabilities.validateCalls)
			}
			if len(repo.createTaskSessionCalls) != 0 {
				t.Fatalf("created %d sessions before admission", len(repo.createTaskSessionCalls))
			}
			if len(repo.createTaskEnvironmentCalls) != 0 {
				t.Fatalf("created %d task environments before admission", len(repo.createTaskEnvironmentCalls))
			}
		})
	}
	t.Run("executor_lookup_failure_is_unresolved_selection", func(t *testing.T) {
		repo := newMockRepository()
		repo.getExecutorFunc = func(context.Context, string) (*models.Executor, error) {
			return nil, errors.New("database unavailable")
		}
		exec := newTestExecutor(t, &mockAgentManager{}, repo)
		exec.SetCapabilities(&denyDockerCapabilities{})

		_, err := exec.PrepareSession(
			context.Background(), &v1.Task{ID: "task-1", WorkspaceID: "workspace-1"},
			"profile-1", "executor-1", "", "",
		)
		if !errors.Is(err, ErrExecutorRuntimeSelectionUnresolved) {
			t.Fatalf("PrepareSession error = %v, want unresolved selection", err)
		}
		if len(repo.createTaskSessionCalls) != 0 {
			t.Fatalf("created %d sessions after a failed executor lookup", len(repo.createTaskSessionCalls))
		}
	})
}

// TestExecutorRuntimeResumeAdmission proves a required-runtime refusal happens
// before stale-runtime cleanup and before the resume request reaches
// credential issuance.
func TestExecutorRuntimeResumeAdmission(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task-1"] = &models.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Task"}
	repo.sessions["session-1"] = &models.TaskSession{
		ID: "session-1", TaskID: "task-1", AgentProfileID: "profile-1",
		ExecutorID: models.ExecutorIDLocalDocker,
		State:      models.TaskSessionStateFailed,
	}
	agentManager := &mockAgentManager{}
	exec := newTestExecutor(t, agentManager, repo)
	exec.SetCapabilities(&denyDockerCapabilities{})

	session, err := repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = exec.ResumeSession(context.Background(), session, true)
	if !errors.Is(err, lifecycle.ErrExecutorRuntimeUnavailable) {
		t.Fatalf("ResumeSession error = %v, want runtime unavailable", err)
	}
	if agentManager.cleanupStaleExecutionCallCount != 0 {
		t.Fatalf("cleanup calls before admission = %d, want 0", agentManager.cleanupStaleExecutionCallCount)
	}
	if got := repo.sessions["session-1"].State; got != models.TaskSessionStateFailed {
		t.Fatalf("session state changed before admission: %s", got)
	}
}

// requestScopedRetainedAdmission refuses a Docker request against retained
// standalone inventory and records the request-scoped validation.
type requestScopedRetainedAdmission struct {
	calls int
}

func (a *requestScopedRetainedAdmission) RequiresCloneURL(string) bool          { return false }
func (a *requestScopedRetainedAdmission) ShouldApplyPreferredShell(string) bool { return false }
func (a *requestScopedRetainedAdmission) ValidateExecutorRuntime(string) error  { return nil }

func (a *requestScopedRetainedAdmission) ValidateExecutionRuntime(
	_ context.Context, _ string, _ string, executorType string,
) error {
	a.calls++
	if executorType == string(models.ExecutorTypeLocalDocker) {
		return lifecycle.ErrExecutorRuntimeMismatch
	}
	return nil
}

// TestLaunchPreparedSessionRejectsRetainedRuntimeBeforeEffects proves the
// prepared-workspace path performs request-scoped retained admission before
// profile cleanup, credential delivery, MCP/env updates, or a start.
func TestLaunchPreparedSessionRejectsRetainedRuntimeBeforeEffects(t *testing.T) {
	for _, startAgent := range []bool{false, true} {
		name := "workspace_only"
		if startAgent {
			name = "start_agent"
		}
		t.Run(name, func(t *testing.T) {
			repo := newMockRepository()
			repo.tasks["task-1"] = &models.Task{ID: "task-1", WorkspaceID: "workspace-1", Title: "Task"}
			repo.sessions["session-1"] = &models.TaskSession{
				ID: "session-1", TaskID: "task-1", AgentProfileID: "profile-1",
				ExecutorID: models.ExecutorIDWorktree, TaskEnvironmentID: "env-1",
				State: models.TaskSessionStateWaitingForInput,
			}
			repo.executorsRunning["session-1"] = &models.ExecutorRunning{
				ID: "running-session-1", SessionID: "session-1", TaskID: "task-1",
				AgentExecutionID: "exec-old", Runtime: agentruntime.RuntimeStandalone,
				ExecutionProfileID: "profile-1", Status: models.ExecutorRunningStatusRunning,
			}
			// The retained session's old selection is Worktree; the incoming
			// request selects Local Docker. Admission must be request-scoped.
			agentManager := &mockAgentManager{}
			exec := newTestExecutor(t, agentManager, repo)
			admission := &requestScopedRetainedAdmission{}
			exec.SetCapabilities(admission)

			_, err := exec.LaunchPreparedSession(
				context.Background(),
				&v1.Task{ID: "task-1", WorkspaceID: "workspace-1"},
				"session-1",
				LaunchOptions{
					AgentProfileID: "profile-1",
					ExecutorID:     models.ExecutorIDLocalDocker,
					Prompt:         "continue",
					StartAgent:     startAgent,
				},
			)
			if !errors.Is(err, lifecycle.ErrExecutorRuntimeMismatch) {
				t.Fatalf("LaunchPreparedSession error = %v, want runtime mismatch", err)
			}
			if admission.calls != 1 {
				t.Fatalf("request-scoped admission calls = %d, want 1", admission.calls)
			}
			if agentManager.setExecutionEnvCallCount != 0 ||
				agentManager.setMcpModeCallCount != 0 ||
				agentManager.stopAgentCallCount != 0 ||
				agentManager.launchAgentCallCount != 0 ||
				agentManager.startAgentProcessCallCount != 0 ||
				agentManager.promptAgentCallCount != 0 {
				t.Fatalf(
					"effects before admission: env=%d mcp=%d stop=%d launch=%d start=%d prompt=%d",
					agentManager.setExecutionEnvCallCount,
					agentManager.setMcpModeCallCount,
					agentManager.stopAgentCallCount,
					agentManager.launchAgentCallCount,
					agentManager.startAgentProcessCallCount,
					agentManager.promptAgentCallCount,
				)
			}
			row := repo.executorsRunning["session-1"]
			if row == nil || row.Runtime != agentruntime.RuntimeStandalone || row.AgentExecutionID != "exec-old" {
				t.Fatalf("retained inventory changed: %+v", row)
			}
		})
	}
}

// TestExplicitExecutorProfileParentFailureIsUnresolved proves a transient
// failure to resolve the explicitly selected profile's parent never leaves the
// workspace-default host runtime selected.
func TestExplicitExecutorProfileParentFailureIsUnresolved(t *testing.T) {
	for _, parentResult := range []string{"error", "not_found"} {
		t.Run(parentResult, func(t *testing.T) {
			repo := newMockRepository()
			repo.executorProfiles["profile-docker"] = &models.ExecutorProfile{
				ID: "profile-docker", ExecutorID: "exec-docker-parent",
			}
			parentCalls := 0
			repo.getExecutorFunc = func(_ context.Context, id string) (*models.Executor, error) {
				if id == "exec-docker-parent" {
					parentCalls++
					if parentResult == "error" {
						return nil, errors.New("transient executor lookup failure")
					}
					return nil, nil
				}
				return &models.Executor{ID: id, Type: models.ExecutorTypeLocal}, nil
			}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			exec.SetCapabilities(&denyDockerCapabilities{})

			_, err := exec.PrepareSession(
				context.Background(),
				&v1.Task{ID: "task-1", WorkspaceID: "workspace-1"},
				"profile-1", models.ExecutorIDLocal, "profile-docker", "",
			)
			if !errors.Is(err, ErrExecutorRuntimeSelectionUnresolved) {
				t.Fatalf("PrepareSession error = %v, want unresolved selection", err)
			}
			if parentCalls != 1 {
				t.Fatalf("parent executor lookups = %d, want 1", parentCalls)
			}
			if len(repo.createTaskSessionCalls) != 0 {
				t.Fatalf("created %d sessions before profile-parent refusal", len(repo.createTaskSessionCalls))
			}
		})
	}
}

// TestConcreteSelectionWithoutRuntimeAdmissionFailsClosed proves a missing
// admission capability refuses a concrete selection instead of allowing
// preparation effects before lifecycle rejects an unavailable runtime.
func TestConcreteSelectionWithoutRuntimeAdmissionFailsClosed(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetCapabilities(nil)

	if err := exec.admitExecutorRuntime(string(models.ExecutorTypeLocalDocker)); !errors.Is(err, ErrExecutorRuntimeAdmissionUnavailable) {
		t.Fatalf("admitExecutorRuntime error = %v, want admission unavailable", err)
	}
	if err := exec.admitExecutorRuntime(""); err != nil {
		t.Fatalf("empty selection error = %v, want nil", err)
	}
	if err := exec.admitRetainedExecutionRuntime(context.Background(), &models.TaskSession{
		ID: "session-without-admission",
	}, string(models.ExecutorTypeLocalDocker)); !errors.Is(err, ErrExecutorRuntimeAdmissionUnavailable) {
		t.Fatalf("admitRetainedExecutionRuntime error = %v, want admission unavailable", err)
	}
}

type recordingRetainedAdmission struct {
	calls []string
}

func (a *recordingRetainedAdmission) RequiresCloneURL(string) bool          { return false }
func (a *recordingRetainedAdmission) ShouldApplyPreferredShell(string) bool { return false }
func (a *recordingRetainedAdmission) ValidateExecutorRuntime(string) error  { return nil }

func (a *recordingRetainedAdmission) ValidateExecutionRuntime(
	_ context.Context, _, _, executorType string,
) error {
	a.calls = append(a.calls, executorType)
	return nil
}

// TestEmptyResolvedTypeStillValidatesRetainedBinding proves an empty resolved
// type is forwarded so lifecycle can validate the retained binding's own type.
func TestEmptyResolvedTypeStillValidatesRetainedBinding(t *testing.T) {
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	admission := &recordingRetainedAdmission{}
	exec.SetCapabilities(admission)

	err := exec.admitRetainedExecutionRuntime(context.Background(), &models.TaskSession{
		ID: "session-legacy", TaskEnvironmentID: "environment-legacy",
	}, "")
	if err != nil {
		t.Fatalf("admitRetainedExecutionRuntime error = %v", err)
	}
	if len(admission.calls) != 1 || admission.calls[0] != "" {
		t.Fatalf("retained admission calls = %v, want one empty type", admission.calls)
	}
}

// TestClassifyLaunchFailureEnvironmentMismatch keeps the runtime-mismatch
// classification while giving an environment-binding refusal its own message.
func TestClassifyLaunchFailureEnvironmentMismatch(t *testing.T) {
	err := &lifecycle.ExecutorRuntimeError{
		Reason:              lifecycle.ExecutorRuntimeReasonMismatch,
		Runtime:             agentruntime.RuntimeStandalone,
		Observed:            agentruntime.RuntimeStandalone,
		Detail:              `retained execution belongs to task environment "env-old", not "env-new"`,
		EnvironmentMismatch: true,
	}
	classification := classifyLaunchFailure(err)
	if classification.code != models.LaunchErrorCategoryGenericLaunchFailure {
		t.Fatalf("category = %q, want generic launch failure", classification.code)
	}
	if !strings.Contains(classification.message, "different task environment") {
		t.Fatalf("message = %q, want environment-specific guidance", classification.message)
	}
}
