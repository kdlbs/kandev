package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// unansweredBundleLister adds the optional unanswered read to the recording
// bundle double.
type unansweredBundleLister struct {
	recordingBundleLister
	unanswered      *models.ClarificationBundlePage
	unansweredOpts  []models.ListUnansweredClarificationsOptions
	unansweredCalls int
}

func (u *unansweredBundleLister) ListUnansweredClarificationBundles(
	_ context.Context, opts models.ListUnansweredClarificationsOptions,
) (*models.ClarificationBundlePage, error) {
	u.unansweredCalls++
	u.unansweredOpts = append(u.unansweredOpts, opts)
	if u.unanswered == nil {
		return &models.ClarificationBundlePage{}, nil
	}
	return u.unanswered, nil
}

var unansweredListBase = time.Date(2026, time.October, 2, 8, 0, 0, 0, time.UTC)

func newUnansweredLister() *unansweredBundleLister {
	return &unansweredBundleLister{
		recordingBundleLister: recordingBundleLister{
			page: &models.ClarificationBundlePage{Bundles: []models.ClarificationBundleSummary{{
				PendingID: "active-1", TaskID: "task-a", SessionID: "session-a", CreatedAt: unansweredListBase.Add(time.Minute),
			}}},
			msgs: map[string][]*models.Message{
				"active-1":     {questionMessage("active-1", "q1", "pending", 0, nil)},
				"unanswered-1": {questionMessage("unanswered-1", "q1", "pending", 0, nil)},
			},
		},
		unanswered: &models.ClarificationBundlePage{Bundles: []models.ClarificationBundleSummary{{
			PendingID: "unanswered-1", TaskID: "task-b", SessionID: "session-b", CreatedAt: unansweredListBase,
		}}},
	}
}

func TestHandleListPendingQuestionsDefaultOmitsUnansweredAndState(t *testing.T) {
	svc, _ := newTestTaskService(t)
	lister := newUnansweredLister()
	h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}

	resp, err := h.handleListPendingQuestions(context.Background(), makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var body listPendingQuestionsResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.Equal(t, 1, body.Count)
	require.Equal(t, "active-1", body.Bundles[0].PendingID)
	require.NotContains(t, string(resp.Payload), `"state"`)
	require.Zero(t, lister.unansweredCalls)
}

func TestHandleListPendingQuestionsIncludeUnansweredMergesAndLabels(t *testing.T) {
	svc, _ := newTestTaskService(t)
	lister := newUnansweredLister()
	h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}

	resp, err := h.handleListPendingQuestions(context.Background(), makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{
		"include_unanswered": true,
	}))
	require.NoError(t, err)
	var body listPendingQuestionsResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.Equal(t, 2, body.Count)
	require.Equal(t, "unanswered-1", body.Bundles[0].PendingID)
	require.Equal(t, pendingQuestionStateUnanswered, body.Bundles[0].State)
	require.Equal(t, "active-1", body.Bundles[1].PendingID)
	require.Equal(t, pendingQuestionStateActive, body.Bundles[1].State)
	require.Len(t, lister.unansweredOpts, 1)
	require.True(t, lister.unansweredOpts[0].AnyWorkspace)
	require.Empty(t, body.NextCursor)
}

func TestHandleListPendingQuestionsIncludeUnansweredPaginatesOneSequence(t *testing.T) {
	svc, _ := newTestTaskService(t)
	lister := newUnansweredLister()
	h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}

	resp, err := h.handleListPendingQuestions(context.Background(), makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{
		"include_unanswered": true, "limit": 1,
	}))
	require.NoError(t, err)
	var body listPendingQuestionsResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.Equal(t, 1, body.Count)
	require.Equal(t, "unanswered-1", body.Bundles[0].PendingID)
	require.NotEmpty(t, body.NextCursor)
	createdAt, pendingID, err := decodeBundleCursor(body.NextCursor)
	require.NoError(t, err)
	require.Equal(t, "unanswered-1", pendingID)
	require.True(t, createdAt.Equal(unansweredListBase))
}

func TestHandleListPendingQuestionsAutomationSeesOwnUnansweredQuestions(t *testing.T) {
	svc, _ := newTestTaskService(t)
	lister := newUnansweredLister()
	lister.page = &models.ClarificationBundlePage{Bundles: []models.ClarificationBundleSummary{{
		PendingID: "self-active", TaskID: "automation-task", SessionID: "automation-session", CreatedAt: unansweredListBase.Add(time.Minute),
	}}}
	lister.unanswered = &models.ClarificationBundlePage{Bundles: []models.ClarificationBundleSummary{{
		PendingID: "unanswered-1", TaskID: "automation-task", SessionID: "automation-session", CreatedAt: unansweredListBase,
	}}}
	h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		AutomationID: "automation-1", WorkspaceID: "ws-1",
		CallerTaskID: "automation-task", CallerSessionID: "automation-session",
		Surface: mcpprofile.SurfaceAutomation,
	})

	resp, err := h.handleListPendingQuestions(ctx, makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{
		"include_unanswered": true,
	}))
	require.NoError(t, err)
	var body listPendingQuestionsResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.Equal(t, 1, body.Count)
	require.Equal(t, "unanswered-1", body.Bundles[0].PendingID)
	require.Equal(t, pendingQuestionStateUnanswered, body.Bundles[0].State)
	require.Equal(t, []string{"ws-1"}, lister.unansweredOpts[0].WorkspaceIDs)
	require.False(t, lister.unansweredOpts[0].AnyWorkspace)
}

func TestHandleListPendingQuestionsUnansweredRespectsCallerWorkspaces(t *testing.T) {
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-owned", Name: "Mine", OwnerID: "user-1"}))
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-other", Name: "Theirs", OwnerID: "user-2"}))
	lister := newUnansweredLister()
	h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}
	scopedCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "user-1"})

	_, err := h.handleListPendingQuestions(scopedCtx, makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{
		"include_unanswered": true, "workspace_id": "ws-other",
	}))
	require.NoError(t, err)
	require.Zero(t, lister.unansweredCalls, "a foreign workspace filter must not reach the unanswered read")

	_, err = h.handleListPendingQuestions(scopedCtx, makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{
		"include_unanswered": true,
	}))
	require.NoError(t, err)
	require.Len(t, lister.unansweredOpts, 1)
	require.Contains(t, lister.unansweredOpts[0].WorkspaceIDs, "ws-owned")
	require.NotContains(t, lister.unansweredOpts[0].WorkspaceIDs, "ws-other")
	require.False(t, lister.unansweredOpts[0].AnyWorkspace)
}

func TestHandleListPendingQuestionsIncludeUnansweredWithoutStoreSupport(t *testing.T) {
	svc, _ := newTestTaskService(t)
	lister := &recordingBundleLister{
		page: &models.ClarificationBundlePage{Bundles: []models.ClarificationBundleSummary{{
			PendingID: "active-1", TaskID: "task-a", SessionID: "session-a", CreatedAt: unansweredListBase,
		}}},
		msgs: map[string][]*models.Message{"active-1": {questionMessage("active-1", "q1", "pending", 0, nil)}},
	}
	h := &Handlers{taskSvc: svc, clarificationBundles: lister, logger: testLogger(t)}
	resp, err := h.handleListPendingQuestions(context.Background(), makeWSMessage(t, ws.ActionMCPListPendingQuestions, map[string]interface{}{
		"include_unanswered": true,
	}))
	require.NoError(t, err)
	var body listPendingQuestionsResponse
	require.NoError(t, json.Unmarshal(resp.Payload, &body))
	require.Equal(t, 1, body.Count)
	require.Equal(t, pendingQuestionStateActive, body.Bundles[0].State)
}
