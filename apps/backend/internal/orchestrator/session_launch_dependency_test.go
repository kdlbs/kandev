package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type launchDependencyReader struct {
	blocked bool
	err     error
}

func (r *launchDependencyReader) DependencyGate(context.Context, string) (bool, string, error) {
	return r.blocked, "pending", r.err
}

func (r *launchDependencyReader) ListDependentTaskIDs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (r *launchDependencyReader) ListPendingDependencyLaunches(
	context.Context,
) ([]taskservice.PendingDependencyLaunch, error) {
	return nil, nil
}

// @covers AC-TASKS-TASK-DEPENDENCIES-001.1
func TestLaunchSession_AutoStartDependencyGate(t *testing.T) {
	tests := []struct {
		name            string
		blocked         bool
		dependencyError error
		wantPrepareOnly bool
	}{
		{
			name:            "unresolved dependency prepares workspace only",
			blocked:         true,
			wantPrepareOnly: true,
		},
		{
			name: "resolved dependency follows normal launch path",
		},
		{
			name:            "dependency read error prepares workspace only",
			dependencyError: errors.New("dependency store unavailable"),
			wantPrepareOnly: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const taskID = "task-focus-gate"
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedAutoStartGateTask(t, repo, taskID)

			taskRepo := newMockTaskRepo()
			taskRepo.tasks[taskID] = &v1.Task{
				ID:          taskID,
				WorkspaceID: "ws1",
				Title:       "Focus gate task",
				Description: "run the task",
				State:       v1.TaskStateCreated,
			}

			var agentLaunches atomic.Int32
			workspaceLaunchDone := make(chan struct{})
			agentProcessStarted := make(chan struct{})
			agentManager := &mockAgentManager{
				launchAgentFunc: func(
					_ context.Context, req *executor.LaunchAgentRequest,
				) (*executor.LaunchAgentResponse, error) {
					if req.StartAgent {
						agentLaunches.Add(1)
					} else {
						close(workspaceLaunchDone)
					}
					return &executor.LaunchAgentResponse{
						AgentExecutionID: "exec-focus-gate",
						Status:           v1.AgentStatusStarting,
					}, nil
				},
				startAgentProcessFunc: func(context.Context, string) error {
					close(agentProcessStarted)
					return nil
				},
			}
			svc := createTestServiceWithScheduler(
				repo, newMockStepGetter(), taskRepo, agentManager,
			)
			svc.SetTaskDependencyReader(&launchDependencyReader{
				blocked: tt.blocked,
				err:     tt.dependencyError,
			})

			response, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
				TaskID:         taskID,
				Intent:         IntentStart,
				AgentProfileID: "profile-focus-gate",
				AutoStart:      true,
			})
			if err != nil {
				t.Fatalf("LaunchSession returned error: %v", err)
			}
			if response == nil {
				t.Fatal("LaunchSession returned nil response")
			}

			if tt.wantPrepareOnly {
				if response.State != string(models.TaskSessionStateCreated) {
					t.Fatalf("response state = %q, want CREATED", response.State)
				}
				if response.SessionID == "" {
					t.Fatal("prepare-only response did not include a session ID")
				}
				if got := agentLaunches.Load(); got != 0 {
					t.Fatalf("agent launches = %d, want 0", got)
				}
				waitForWorkspaceLaunch(t, workspaceLaunchDone)
				assertTaskWasNotScheduled(t, repo, taskRepo, taskID)
				return
			}

			if got := agentLaunches.Load(); got != 1 {
				t.Fatalf("agent launches = %d, want 1", got)
			}
			if response.AgentExecutionID == "" {
				t.Fatal("normal launch response did not include an agent execution ID")
			}
			select {
			case <-agentProcessStarted:
			case <-time.After(2 * time.Second):
				t.Fatal("normal launch did not start the agent process")
			}
		})
	}
}

func seedAutoStartGateTask(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
}, taskID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws1", Name: "Focus gate workspace", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf1", WorkspaceID: "ws1", Name: "Focus gate workflow", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Focus gate task", Description: "run the task", State: v1.TaskStateCreated,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
}

func waitForWorkspaceLaunch(t *testing.T, launched <-chan struct{}) {
	t.Helper()
	select {
	case <-launched:
	case <-time.After(2 * time.Second):
		t.Fatal("workspace-only prepare did not launch workspace")
	}
}

func assertTaskWasNotScheduled(t *testing.T, repo interface {
	GetTask(context.Context, string) (*models.Task, error)
}, taskRepo *mockTaskRepo, taskID string) {
	t.Helper()
	task, err := repo.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State == v1.TaskStateScheduling || task.State == v1.TaskStateInProgress {
		t.Fatalf("task state = %q, must not be scheduled or in progress", task.State)
	}

	taskRepo.mu.Lock()
	defer taskRepo.mu.Unlock()
	for _, state := range taskRepo.stateHistory[taskID] {
		if state == v1.TaskStateScheduling || state == v1.TaskStateInProgress {
			t.Fatalf("task state history contains %q", state)
		}
	}
}
