package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type dependencyTestLookup struct{}

func (dependencyTestLookup) GetRepository(_ context.Context, id string) (string, string, bool) {
	switch id {
	case "repo-valid":
		return "ws-1", "main", true
	case "repo-other":
		return "ws-2", "main", true
	default:
		return "", "", false
	}
}

type dependencyTestValidator struct{}

func (dependencyTestValidator) WorkflowStepBelongs(_ context.Context, workspaceID, workflowID, stepID string) (bool, error) {
	return workspaceID == "ws-1" && workflowID == "workflow-valid" && stepID == "step-valid", nil
}
func (dependencyTestValidator) AgentProfileBelongs(_ context.Context, workspaceID, profileID string) (bool, error) {
	return workspaceID == "ws-1" && profileID == "agent-valid", nil
}
func (dependencyTestValidator) ExecutorProfileBelongs(_ context.Context, workspaceID, profileID string) (bool, error) {
	return workspaceID == "ws-1" && profileID == "executor-valid", nil
}

func newDependencyTestService(t *testing.T) (*Service, *Store) {
	t.Helper()
	store := newTestStore(t)
	svc := NewService("", nil, "none", nil, newTestLogger(t))
	svc.SetStore(store)
	svc.SetRepositoryLookup(dependencyTestLookup{})
	svc.SetWatchDependencyValidator(dependencyTestValidator{})
	return svc, store
}

func validReviewWatchRequest() *CreateReviewWatchRequest {
	return &CreateReviewWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "workflow-valid", WorkflowStepID: "step-valid",
		AgentProfileID: "agent-valid", ExecutorProfileID: "executor-valid", RepositoryID: "repo-valid",
	}
}

func validIssueWatchRequest() *CreateIssueWatchRequest {
	return &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "workflow-valid", WorkflowStepID: "step-valid",
		AgentProfileID: "agent-valid", ExecutorProfileID: "executor-valid", RepositoryID: "repo-valid",
	}
}

func TestCreateGitLabWatchesRejectMissingAndCrossWorkspaceDependencies(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CreateReviewWatchRequest, *CreateIssueWatchRequest)
	}{
		{"cross-workspace workflow", func(r *CreateReviewWatchRequest, i *CreateIssueWatchRequest) {
			r.WorkflowID, i.WorkflowID = "workflow-other", "workflow-other"
		}},
		{"missing step", func(r *CreateReviewWatchRequest, i *CreateIssueWatchRequest) {
			r.WorkflowStepID, i.WorkflowStepID = "step-missing", "step-missing"
		}},
		{"missing agent profile", func(r *CreateReviewWatchRequest, i *CreateIssueWatchRequest) {
			r.AgentProfileID, i.AgentProfileID = "agent-missing", "agent-missing"
		}},
		{"missing executor profile", func(r *CreateReviewWatchRequest, i *CreateIssueWatchRequest) {
			r.ExecutorProfileID, i.ExecutorProfileID = "executor-missing", "executor-missing"
		}},
		{"cross-workspace repository", func(r *CreateReviewWatchRequest, i *CreateIssueWatchRequest) {
			r.RepositoryID, i.RepositoryID = "repo-other", "repo-other"
		}},
		{"missing repository", func(r *CreateReviewWatchRequest, i *CreateIssueWatchRequest) {
			r.RepositoryID, i.RepositoryID = "repo-missing", "repo-missing"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, _ := newDependencyTestService(t)
			reviewReq, issueReq := validReviewWatchRequest(), validIssueWatchRequest()
			test.mutate(reviewReq, issueReq)
			if _, err := svc.CreateReviewWatch(context.Background(), reviewReq); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("review error = %v", err)
			}
			if _, err := svc.CreateIssueWatch(context.Background(), issueReq); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("issue error = %v", err)
			}
		})
	}
}

func TestEditGitLabWatchesRejectMissingAndCrossWorkspaceDependencies(t *testing.T) {
	tests := []struct {
		name        string
		reviewPatch *UpdateReviewWatchRequest
		issuePatch  *UpdateIssueWatchRequest
	}{
		{"cross-workspace workflow", &UpdateReviewWatchRequest{WorkflowID: strptr("workflow-other")}, &UpdateIssueWatchRequest{WorkflowID: strptr("workflow-other")}},
		{"missing step", &UpdateReviewWatchRequest{WorkflowStepID: strptr("step-missing")}, &UpdateIssueWatchRequest{WorkflowStepID: strptr("step-missing")}},
		{"missing agent profile", &UpdateReviewWatchRequest{AgentProfileID: strptr("agent-missing")}, &UpdateIssueWatchRequest{AgentProfileID: strptr("agent-missing")}},
		{"missing executor profile", &UpdateReviewWatchRequest{ExecutorProfileID: strptr("executor-missing")}, &UpdateIssueWatchRequest{ExecutorProfileID: strptr("executor-missing")}},
		{"cross-workspace repository", &UpdateReviewWatchRequest{RepositoryID: strptr("repo-other")}, &UpdateIssueWatchRequest{RepositoryID: strptr("repo-other")}},
		{"missing repository", &UpdateReviewWatchRequest{RepositoryID: strptr("repo-missing")}, &UpdateIssueWatchRequest{RepositoryID: strptr("repo-missing")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, store := newDependencyTestService(t)
			review := validReviewWatchRow()
			issue := validIssueWatchRow()
			if err := store.CreateReviewWatch(context.Background(), review); err != nil {
				t.Fatalf("seed review: %v", err)
			}
			if err := store.CreateIssueWatch(context.Background(), issue); err != nil {
				t.Fatalf("seed issue: %v", err)
			}
			if err := svc.UpdateReviewWatch(context.Background(), review.ID, test.reviewPatch); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("review error = %v", err)
			}
			if err := svc.UpdateIssueWatch(context.Background(), issue.ID, test.issuePatch); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("issue error = %v", err)
			}
		})
	}
}

func TestEditGitLabWatchesZeroMaxInflightClearsExistingCap(t *testing.T) {
	svc, store := newDependencyTestService(t)
	limit := 3
	clearLimit := 0
	review := validReviewWatchRow()
	review.MaxInflightTasks = &limit
	issue := validIssueWatchRow()
	issue.MaxInflightTasks = &limit
	if err := store.CreateReviewWatch(context.Background(), review); err != nil {
		t.Fatalf("seed review: %v", err)
	}
	if err := store.CreateIssueWatch(context.Background(), issue); err != nil {
		t.Fatalf("seed issue: %v", err)
	}

	if err := svc.UpdateReviewWatch(context.Background(), review.ID, &UpdateReviewWatchRequest{MaxInflightTasks: &clearLimit}); err != nil {
		t.Fatalf("clear review max inflight: %v", err)
	}
	if err := svc.UpdateIssueWatch(context.Background(), issue.ID, &UpdateIssueWatchRequest{MaxInflightTasks: &clearLimit}); err != nil {
		t.Fatalf("clear issue max inflight: %v", err)
	}
	updatedReview, err := svc.GetReviewWatch(context.Background(), review.ID)
	if err != nil {
		t.Fatalf("get review: %v", err)
	}
	updatedIssue, err := svc.GetIssueWatch(context.Background(), issue.ID)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if updatedReview.MaxInflightTasks != nil || updatedIssue.MaxInflightTasks != nil {
		t.Fatalf("limits not cleared: review=%v issue=%v", updatedReview.MaxInflightTasks, updatedIssue.MaxInflightTasks)
	}
}

func TestCreateGitLabWatchesStillRejectZeroMaxInflight(t *testing.T) {
	svc, _ := newDependencyTestService(t)
	zero := 0
	review := validReviewWatchRequest()
	review.MaxInflightTasks = &zero
	issue := validIssueWatchRequest()
	issue.MaxInflightTasks = &zero
	if _, err := svc.CreateReviewWatch(context.Background(), review); err == nil {
		t.Fatal("review create accepted a zero max inflight cap")
	}
	if _, err := svc.CreateIssueWatch(context.Background(), issue); err == nil {
		t.Fatal("issue create accepted a zero max inflight cap")
	}
}

func validReviewWatchRow() *ReviewWatch {
	return &ReviewWatch{WorkspaceID: "ws-1", WorkflowID: "workflow-valid", WorkflowStepID: "step-valid", AgentProfileID: "agent-valid", ExecutorProfileID: "executor-valid", RepositoryID: "repo-valid", BaseBranch: "main", Enabled: true}
}
func validIssueWatchRow() *IssueWatch {
	return &IssueWatch{WorkspaceID: "ws-1", WorkflowID: "workflow-valid", WorkflowStepID: "step-valid", AgentProfileID: "agent-valid", ExecutorProfileID: "executor-valid", RepositoryID: "repo-valid", BaseBranch: "main", Enabled: true}
}
func strptr(value string) *string { return &value }

func TestWatchDependencyControllerErrorDoesNotDiscloseResource(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	secretID := "repo-sensitive-123"
	if !httpRespondError(ctx, fmt.Errorf("%w: repository %s belongs elsewhere", ErrInvalidConfig, secretID)) {
		t.Fatal("error was not handled")
	}
	if recorder.Code != 400 || strings.Contains(recorder.Body.String(), secretID) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
