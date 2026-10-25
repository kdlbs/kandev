package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

func TestExactProfileAssignerRejectsChangedProfileRevision(t *testing.T) {
	repo := &exactAssignmentRepo{assignment: &models.ExactProfileAssignment{
		TaskID: "task-1", WorkspaceID: "workspace-1", AgentProfileID: "profile-1",
		ProfileRevision: 1, Generation: 1, Active: true,
	}}
	assigner := ExactProfileAssigner{
		Assignments: repo,
		Profiles: exactProfileLookupFunc(func(context.Context, string) (*executor.AgentProfileInfo, error) {
			return &executor.AgentProfileInfo{
				ProfileID: "profile-1", WorkspaceID: "workspace-1", Enabled: true,
				Revision: 2,
			}, nil
		}),
	}

	_, err := assigner.Resolve(context.Background(), "task-1", "workspace-1")
	if !errors.Is(err, ErrExactProfileAssignmentInvalid) {
		t.Fatalf("Resolve error = %v, want invalid exact profile assignment", err)
	}
}

func TestAssignExactTaskProfileRequiresTaskPromptScope(t *testing.T) {
	denied := errors.New("task prompt denied")
	repo := &exactAssignmentWriteProbeRepo{}
	service := &Service{repo: repo, agentManager: &mockAgentManager{},
		taskAccessCheck: func(context.Context, string) error { return nil },
		taskPromptCheck: func(context.Context, string) error { return denied }}
	if _, err := service.AssignExactTaskProfile(context.Background(), "task-1", "profile-1", 1); !errors.Is(err, denied) {
		t.Fatalf("assignment error = %v, want task prompt denial", err)
	}
	if repo.wrote {
		t.Fatal("denied assignment reached repository writer")
	}
}

func TestAssignExactTaskProfileRejectsMissingTask(t *testing.T) {
	repo := &exactAssignmentWriteProbeRepo{}
	service := &Service{repo: repo, agentManager: &mockAgentManager{},
		taskAccessCheck: func(context.Context, string) error { return nil },
		taskPromptCheck: func(context.Context, string) error { return nil }}

	_, err := service.AssignExactTaskProfile(context.Background(), "missing-task", "profile-1", 1)
	if !errors.Is(err, ErrExactProfileAssignmentInvalid) {
		t.Fatalf("assignment error = %v, want invalid exact profile assignment", err)
	}
	if repo.wrote {
		t.Fatal("missing task reached repository writer")
	}
}

func TestExactProfileReceiptWriteErrorIsReturned(t *testing.T) {
	writeErr := errors.New("receipt unavailable")
	service := &Service{repo: &exactReceiptErrorRepo{err: writeErr}}
	err := service.recordExactProfileLaunchReceipt(context.Background(), "task-1", "session-1",
		&ExactProfileLaunchDecision{AgentProfileID: "profile-1", Generation: 1, Revision: 2, Model: "model-1"},
		"model-1", nil)
	if !errors.Is(err, writeErr) {
		t.Fatalf("receipt error = %v, want storage failure", err)
	}
}

func TestExactProfileLaunchReceiptStoresOnlyBoundedFailureCategory(t *testing.T) {
	repo := &exactReceiptCaptureRepo{}
	service := &Service{repo: repo}
	launchErr := errors.New("provider response from /private/worktree with credential detail")

	if err := service.recordExactProfileLaunchReceipt(context.Background(), "task-1", "session-1",
		&ExactProfileLaunchDecision{AgentProfileID: "profile-1", Generation: 1, Revision: 2, Model: "model-1"},
		"model-1", launchErr); err != nil {
		t.Fatalf("record receipt: %v", err)
	}
	if repo.receipt == nil {
		t.Fatal("receipt was not recorded")
	}
	if repo.receipt.FailureReason != "launch_failed" {
		t.Fatalf("failure reason = %q, want bounded category %q", repo.receipt.FailureReason, "launch_failed")
	}
	if strings.Contains(repo.receipt.FailureReason, "private/worktree") || strings.Contains(repo.receipt.FailureReason, "credential") {
		t.Fatalf("receipt retained raw launch error: %q", repo.receipt.FailureReason)
	}
}

type exactReceiptCaptureRepo struct {
	sessionExecutorStore
	receipt *models.ExactProfileLaunchReceipt
}

func (r *exactReceiptCaptureRepo) RecordExactProfileLaunchReceipt(_ context.Context, receipt *models.ExactProfileLaunchReceipt) (bool, error) {
	r.receipt = receipt
	return true, nil
}

type exactReceiptErrorRepo struct {
	sessionExecutorStore
	err error
}

func (r *exactReceiptErrorRepo) RecordExactProfileLaunchReceipt(context.Context, *models.ExactProfileLaunchReceipt) (bool, error) {
	return false, r.err
}

type exactAssignmentWriteProbeRepo struct {
	sessionExecutorStore
	wrote bool
}

func (*exactAssignmentWriteProbeRepo) GetTask(context.Context, string) (*models.Task, error) {
	return nil, nil
}

func (r *exactAssignmentWriteProbeRepo) GetExactProfileAssignment(context.Context, string) (*models.ExactProfileAssignment, error) {
	return nil, nil
}

func (r *exactAssignmentWriteProbeRepo) AssignExactProfileAssignmentIfProfileCurrent(context.Context, *models.ExactProfileAssignment) (bool, error) {
	r.wrote = true
	return true, nil
}

type exactAssignmentRepo struct {
	assignment *models.ExactProfileAssignment
}

func (r *exactAssignmentRepo) GetExactProfileAssignment(_ context.Context, _ string) (*models.ExactProfileAssignment, error) {
	return r.assignment, nil
}

type exactProfileLookupFunc func(context.Context, string) (*executor.AgentProfileInfo, error)

func (f exactProfileLookupFunc) ResolveAgentProfile(ctx context.Context, profileID string) (*executor.AgentProfileInfo, error) {
	return f(ctx, profileID)
}
