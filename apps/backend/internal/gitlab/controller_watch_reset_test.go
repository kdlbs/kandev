package gitlab

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestReviewWatchMutationGuardHidesCrossWorkspaceWatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	watch := &ReviewWatch{
		WorkspaceID: "workspace-a", WorkflowID: "wf", WorkflowStepID: "step",
		AgentProfileID: "agent", ExecutorProfileID: "executor", Enabled: true,
	}
	if err := store.CreateReviewWatch(context.Background(), watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	svc := NewService("", nil, "none", nil, newTestLogger(t))
	svc.SetStore(store)
	controller := NewController(svc, newTestLogger(t))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("PATCH", "/api/v1/gitlab/watches/review/"+watch.ID+"?workspace_id=workspace-b", nil)
	ctx.Params = gin.Params{{Key: "id", Value: watch.ID}}

	if controller.requireReviewWatchInWorkspace(ctx, watch.ID) {
		t.Fatal("cross-workspace watch unexpectedly authorized")
	}
	if recorder.Code != 404 {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestWatchDeleteGuardsAllowOwnedTombstoneRetries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestStore(t)
	review := &ReviewWatch{WorkspaceID: "workspace-a", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "executor", Enabled: true}
	issue := &IssueWatch{WorkspaceID: "workspace-a", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "executor", Enabled: true}
	if err := store.CreateReviewWatch(t.Context(), review); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateIssueWatch(t.Context(), issue); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginReviewWatchDelete(t.Context(), review.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginIssueWatchDelete(t.Context(), issue.ID); err != nil {
		t.Fatal(err)
	}
	service := NewService("", nil, AuthMethodNone, nil, newTestLogger(t))
	service.SetStore(store)
	controller := NewController(service, newTestLogger(t))

	for _, test := range []struct {
		name  string
		id    string
		guard func(*gin.Context, string) bool
	}{{"review", review.ID, controller.requireReviewWatchDeleteInWorkspace}, {"issue", issue.ID, controller.requireIssueWatchDeleteInWorkspace}} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("DELETE", "/?workspace_id=workspace-a", nil)
			if !test.guard(ctx, test.id) {
				t.Fatalf("owned tombstone retry rejected: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			foreignRecorder := httptest.NewRecorder()
			foreign, _ := gin.CreateTestContext(foreignRecorder)
			foreign.Request = httptest.NewRequest("DELETE", "/?workspace_id=workspace-b", nil)
			if test.guard(foreign, test.id) || foreignRecorder.Code != 404 {
				t.Fatalf("foreign tombstone disclosed: status=%d body=%s", foreignRecorder.Code, foreignRecorder.Body.String())
			}
		})
	}
}
