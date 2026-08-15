package github

import (
	"context"
	"errors"
	"fmt"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type authorizeTaskPRMutationCase struct {
	name             string
	association      *TaskPR
	taskStore        *fakeTaskIssueStore
	workspaceID      string
	authorizeErr     error
	wantErr          error
	wantAuthorized   string
	wantResolved     string
	wantTaskGetCalls int
}

func TestAuthorizeTaskPRMutationWorkspace(t *testing.T) {
	tests := []authorizeTaskPRMutationCase{
		{
			name:           "modern uses stored workspace without task lookup",
			association:    &TaskPR{WorkspaceID: "ws-modern"},
			taskStore:      &fakeTaskIssueStore{task: &taskmodels.Task{WorkspaceID: "ws-other"}},
			workspaceID:    "ws-modern",
			wantAuthorized: "ws-modern",
			wantResolved:   "ws-modern",
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
			taskStore:        &fakeTaskIssueStore{task: &taskmodels.Task{ID: "task-legacy", WorkspaceID: "ws-legacy"}},
			workspaceID:      "ws-legacy",
			wantAuthorized:   "ws-legacy",
			wantResolved:     "ws-legacy",
			wantTaskGetCalls: 1,
		},
		{
			name:             "legacy mismatch is not authorized",
			association:      &TaskPR{TaskID: "task-legacy"},
			taskStore:        &fakeTaskIssueStore{task: &taskmodels.Task{ID: "task-legacy", WorkspaceID: "ws-owner"}},
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
			taskStore:        &fakeTaskIssueStore{returnNilTask: true},
			workspaceID:      "ws-supplied",
			wantErr:          ErrTaskPRNotFound,
			wantTaskGetCalls: 1,
		},
		{
			name:             "task not found sentinel fails closed",
			association:      &TaskPR{TaskID: "task-legacy"},
			taskStore:        &fakeTaskIssueStore{taskErr: fmt.Errorf("lookup task: %w", ErrTaskNotFound)},
			workspaceID:      "ws-supplied",
			wantErr:          ErrTaskPRNotFound,
			wantTaskGetCalls: 1,
		},
		{
			name:             "blank task workspace fails closed",
			association:      &TaskPR{TaskID: "task-legacy"},
			taskStore:        &fakeTaskIssueStore{task: &taskmodels.Task{ID: "task-legacy"}},
			workspaceID:      "ws-supplied",
			wantErr:          ErrTaskPRNotFound,
			wantTaskGetCalls: 1,
		},
		{
			name:             "unrelated task lookup error is preserved",
			association:      &TaskPR{TaskID: "task-legacy"},
			taskStore:        &fakeTaskIssueStore{taskErr: lookupErr},
			workspaceID:      "ws-supplied",
			wantErr:          lookupErr,
			wantTaskGetCalls: 1,
		},
		{
			name:             "workspace authorizer error is preserved",
			association:      &TaskPR{TaskID: "task-legacy"},
			taskStore:        &fakeTaskIssueStore{task: &taskmodels.Task{ID: "task-legacy", WorkspaceID: "ws-legacy"}},
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
		{
			name:        "blank task ID fails closed before lookup",
			association: &TaskPR{TaskID: "  "},
			taskStore:   &fakeTaskIssueStore{task: &taskmodels.Task{ID: "task-legacy", WorkspaceID: "ws-owner"}},
			workspaceID: "ws-owner",
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
			resolvedWorkspace, err := svc.authorizeTaskPRMutation(context.Background(), tt.association, tt.workspaceID)
			assertTaskPRAuthorizationResult(t, tt, resolvedWorkspace, err, authorized)
		})
	}
}

func assertTaskPRAuthorizationResult(t *testing.T, tt authorizeTaskPRMutationCase, resolvedWorkspace string, err error, authorized string) {
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
	if resolvedWorkspace != tt.wantResolved {
		t.Fatalf("resolved workspace = %q, want %q", resolvedWorkspace, tt.wantResolved)
	}
	if tt.taskStore != nil && tt.taskStore.getCalls != tt.wantTaskGetCalls {
		t.Fatalf("GetTask calls = %d, want %d", tt.taskStore.getCalls, tt.wantTaskGetCalls)
	}
}
