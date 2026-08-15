package github

import (
	"context"
	"errors"
	"fmt"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type recordingTaskPRTaskStore struct {
	task     *taskmodels.Task
	err      error
	getCalls int
}

func (s *recordingTaskPRTaskStore) GetTask(context.Context, string) (*taskmodels.Task, error) {
	s.getCalls++
	return s.task, s.err
}

func (s *recordingTaskPRTaskStore) ListTaskRepositories(context.Context, string) ([]*taskmodels.TaskRepository, error) {
	return nil, nil
}

func (s *recordingTaskPRTaskStore) GetRepository(context.Context, string) (*taskmodels.Repository, error) {
	return nil, nil
}

func (s *recordingTaskPRTaskStore) UpdateTaskMetadata(context.Context, string, map[string]interface{}) (*taskmodels.Task, error) {
	return s.task, s.err
}

type authorizeTaskPRMutationCase struct {
	name             string
	association      *TaskPR
	taskStore        *recordingTaskPRTaskStore
	workspaceID      string
	authorizeErr     error
	wantErr          error
	wantAuthorized   string
	wantTaskGetCalls int
}

func TestAuthorizeTaskPRMutationWorkspace(t *testing.T) {
	tests := []authorizeTaskPRMutationCase{
		{
			name:           "modern uses stored workspace without task lookup",
			association:    &TaskPR{WorkspaceID: "ws-modern"},
			taskStore:      &recordingTaskPRTaskStore{task: &taskmodels.Task{WorkspaceID: "ws-other"}},
			workspaceID:    "ws-modern",
			wantAuthorized: "ws-modern",
		},
		{
			name:        "modern mismatch is not authorized",
			association: &TaskPR{WorkspaceID: "ws-modern"},
			workspaceID: "ws-other",
			wantErr:     ErrTaskPRNotFound,
		},
		{
			name:             "legacy uses owning task workspace",
			association:      &TaskPR{TaskID: "task-legacy"},
			taskStore:        &recordingTaskPRTaskStore{task: &taskmodels.Task{WorkspaceID: "ws-legacy"}},
			workspaceID:      "ws-legacy",
			wantAuthorized:   "ws-legacy",
			wantTaskGetCalls: 1,
		},
		{
			name:             "legacy mismatch is not authorized",
			association:      &TaskPR{TaskID: "task-legacy"},
			taskStore:        &recordingTaskPRTaskStore{task: &taskmodels.Task{WorkspaceID: "ws-owner"}},
			workspaceID:      "ws-other",
			wantErr:          ErrTaskPRNotFound,
			wantTaskGetCalls: 1,
		},
	}
	runAuthorizeTaskPRMutationCases(t, tests)
}

func TestAuthorizeTaskPRMutationWorkspaceFailures(t *testing.T) {
	lookupErr := errors.New("task store unavailable")
	authorizeErr := errors.New("workspace denied")
	tests := []authorizeTaskPRMutationCase{
		{
			name:        "missing task resolver fails closed",
			association: &TaskPR{TaskID: "task-legacy"},
			workspaceID: "ws-supplied",
			wantErr:     ErrTaskPRNotFound,
		},
		{
			name:             "absent task fails closed",
			association:      &TaskPR{TaskID: "task-legacy"},
			taskStore:        &recordingTaskPRTaskStore{},
			workspaceID:      "ws-supplied",
			wantErr:          ErrTaskPRNotFound,
			wantTaskGetCalls: 1,
		},
		{
			name:             "task not found sentinel fails closed",
			association:      &TaskPR{TaskID: "task-legacy"},
			taskStore:        &recordingTaskPRTaskStore{err: fmt.Errorf("lookup task: %w", ErrTaskNotFound)},
			workspaceID:      "ws-supplied",
			wantErr:          ErrTaskPRNotFound,
			wantTaskGetCalls: 1,
		},
		{
			name:             "blank task workspace fails closed",
			association:      &TaskPR{TaskID: "task-legacy"},
			taskStore:        &recordingTaskPRTaskStore{task: &taskmodels.Task{}},
			workspaceID:      "ws-supplied",
			wantErr:          ErrTaskPRNotFound,
			wantTaskGetCalls: 1,
		},
		{
			name:             "unrelated task lookup error is preserved",
			association:      &TaskPR{TaskID: "task-legacy"},
			taskStore:        &recordingTaskPRTaskStore{err: lookupErr},
			workspaceID:      "ws-supplied",
			wantErr:          lookupErr,
			wantTaskGetCalls: 1,
		},
		{
			name:             "workspace authorizer error is preserved",
			association:      &TaskPR{TaskID: "task-legacy"},
			taskStore:        &recordingTaskPRTaskStore{task: &taskmodels.Task{WorkspaceID: "ws-legacy"}},
			workspaceID:      "ws-legacy",
			authorizeErr:     authorizeErr,
			wantErr:          authorizeErr,
			wantAuthorized:   "ws-legacy",
			wantTaskGetCalls: 1,
		},
		{
			name:        "nil association fails closed",
			workspaceID: "ws-any",
			wantErr:     ErrTaskPRNotFound,
		},
	}
	runAuthorizeTaskPRMutationCases(t, tests)
}

func runAuthorizeTaskPRMutationCases(t *testing.T, tests []authorizeTaskPRMutationCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(nil, AuthMethodNone, nil, nil, nil, testLogger(t))
			if tt.taskStore != nil {
				svc.SetTaskIssueStore(tt.taskStore)
			}
			var authorized string
			svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
				authorized = workspaceID
				return tt.authorizeErr
			})
			err := svc.authorizeTaskPRMutation(context.Background(), tt.association, tt.workspaceID)
			assertTaskPRAuthorizationResult(t, tt, err, authorized)
		})
	}
}

func assertTaskPRAuthorizationResult(t *testing.T, tt authorizeTaskPRMutationCase, err error, authorized string) {
	t.Helper()
	if tt.wantErr == nil && err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
		t.Fatalf("error = %v, want %v", err, tt.wantErr)
	}
	if authorized != tt.wantAuthorized {
		t.Fatalf("authorized workspace = %q, want %q", authorized, tt.wantAuthorized)
	}
	if tt.taskStore != nil && tt.taskStore.getCalls != tt.wantTaskGetCalls {
		t.Fatalf("GetTask calls = %d, want %d", tt.taskStore.getCalls, tt.wantTaskGetCalls)
	}
}
