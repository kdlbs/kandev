package handlers

// Coverage for the WS-layer handlers behind list_review_findings_kandev and
// resolve_review_finding_kandev (REQ-TWS-003 / REQ-TWS-004).

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// errNotVisibleDistinctive is a distinctive authorizer-denial message, so a
// test asserting it appears in an error result fails against a hardcoded or
// generic denial string.
var errNotVisibleDistinctive = errors.New("distinctive: workspace not visible to caller")

func publishOneFinding(t *testing.T, h *Handlers, reviewSvc *service.ReviewService, taskID string) string {
	t.Helper()
	_, findings, err := reviewSvc.PublishFindings(context.Background(), service.PublishFindingsRequest{
		TaskID: taskID,
		Findings: []service.ReviewFindingInput{{
			FilePath: "a.go", StartLine: 1, EndLine: 1, Severity: "minor",
			Category: "c", Title: "t", Body: "b",
		}},
	})
	require.NoError(t, err)
	return findings[0].ID
}

func decodeListReviewFindingsResponse(t *testing.T, resp *ws.Message) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	return out
}

func TestHandleListReviewFindings_RequiresTaskID(t *testing.T) {
	h, _, _ := newReviewHandlers(t)
	msg := makeWSMessage(t, ws.ActionMCPListReviewFindings, map[string]interface{}{})
	resp, err := h.handleListReviewFindings(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleListReviewFindings_EmptyTaskReturnsEmptySuccess(t *testing.T) {
	h, _, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-lrf-empty")

	msg := makeWSMessage(t, ws.ActionMCPListReviewFindings, map[string]interface{}{"task_id": "task-lrf-empty"})
	resp, err := h.handleListReviewFindings(context.Background(), msg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)

	body := decodeListReviewFindingsResponse(t, resp)
	require.Equal(t, []interface{}{}, body["findings"])
	require.InDelta(t, 0, body["total_matched"], 0.001)
	require.Equal(t, false, body["truncated"])
}

func TestHandleListReviewFindings_ResolvedAtNullOnOpenFinding(t *testing.T) {
	h, reviewSvc, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-lrf-null")
	publishOneFinding(t, h, reviewSvc, "task-lrf-null")

	msg := makeWSMessage(t, ws.ActionMCPListReviewFindings, map[string]interface{}{"task_id": "task-lrf-null"})
	resp, err := h.handleListReviewFindings(context.Background(), msg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)

	// Assert against raw JSON: an "omitempty"-tagged field would drop the key
	// rather than render null, and this decode would silently treat both the
	// same by leaving the map key absent.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Payload, &raw))
	var findings []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw["findings"], &findings))
	require.Len(t, findings, 1)
	resolvedAt, ok := findings[0]["resolved_at"]
	require.True(t, ok, "resolved_at key must be present")
	require.Equal(t, "null", string(resolvedAt))
}

func TestHandleListReviewFindings_StatusAndSeverityFilters(t *testing.T) {
	h, reviewSvc, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-lrf-filter")
	id := publishOneFinding(t, h, reviewSvc, "task-lrf-filter")
	_, err := reviewSvc.UpdateFindingStatus(context.Background(), id, "resolved")
	require.NoError(t, err)

	msg := makeWSMessage(t, ws.ActionMCPListReviewFindings, map[string]interface{}{
		"task_id": "task-lrf-filter", "status": "RESOLVED",
	})
	resp, err := h.handleListReviewFindings(context.Background(), msg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)
	body := decodeListReviewFindingsResponse(t, resp)
	require.Len(t, body["findings"], 1)

	badSeverity := makeWSMessage(t, ws.ActionMCPListReviewFindings, map[string]interface{}{
		"task_id": "task-lrf-filter", "severity": "urgent",
	})
	resp, err = h.handleListReviewFindings(context.Background(), badSeverity)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleListReviewFindings_AuthorizationDeniedIsForbidden(t *testing.T) {
	h, reviewSvc, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-lrf-denied")

	reviewSvc.SetTaskAuthorizer(func(_ context.Context, _ string) error {
		return errNotVisibleDistinctive
	})

	msg := makeWSMessage(t, ws.ActionMCPListReviewFindings, map[string]interface{}{"task_id": "task-lrf-denied"})
	resp, err := h.handleListReviewFindings(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)

	var ep ws.ErrorPayload
	require.NoError(t, json.Unmarshal(resp.Payload, &ep))
	require.Contains(t, ep.Message, errNotVisibleDistinctive.Error())
}

func TestHandleResolveReviewFinding_RequiresFindingID(t *testing.T) {
	h, _, _ := newReviewHandlers(t)
	msg := makeWSMessage(t, ws.ActionMCPResolveReviewFinding, map[string]interface{}{"status": "resolved"})
	resp, err := h.handleResolveReviewFinding(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleResolveReviewFinding_RejectsUnknownStatus(t *testing.T) {
	h, reviewSvc, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-rrf-badstatus")
	id := publishOneFinding(t, h, reviewSvc, "task-rrf-badstatus")

	msg := makeWSMessage(t, ws.ActionMCPResolveReviewFinding, map[string]interface{}{
		"finding_id": id, "status": "archived",
	})
	resp, err := h.handleResolveReviewFinding(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleResolveReviewFinding_UnknownFindingNotFound(t *testing.T) {
	h, _, _ := newReviewHandlers(t)
	msg := makeWSMessage(t, ws.ActionMCPResolveReviewFinding, map[string]interface{}{
		"finding_id": "does-not-exist", "status": "resolved",
	})
	resp, err := h.handleResolveReviewFinding(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)
}

func TestHandleResolveReviewFinding_ResolvesAndReturnsFindingEnvelope(t *testing.T) {
	h, reviewSvc, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-rrf-ok")
	id := publishOneFinding(t, h, reviewSvc, "task-rrf-ok")

	msg := makeWSMessage(t, ws.ActionMCPResolveReviewFinding, map[string]interface{}{
		"finding_id": id, "status": "resolved",
	})
	resp, err := h.handleResolveReviewFinding(context.Background(), msg)
	require.NoError(t, err)
	requireWSSuccess(t, resp)

	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	findingRaw, ok := out["finding"]
	require.True(t, ok, "response must carry a top-level finding key")
	var finding map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(findingRaw, &finding))
	require.Equal(t, `"resolved"`, string(finding["status"]))
	require.NotEqual(t, "null", string(finding["resolved_at"]))
}

func TestHandleResolveReviewFinding_UnreachableTaskGetsSameNotFoundAsUnknown(t *testing.T) {
	h, reviewSvc, _ := newReviewHandlers(t)
	seedReviewHandlerTask(t, h, "task-rrf-unreach")
	id := publishOneFinding(t, h, reviewSvc, "task-rrf-unreach")

	reviewSvc.SetTaskAuthorizer(func(_ context.Context, _ string) error {
		return errNotVisibleDistinctive
	})

	unreachable := makeWSMessage(t, ws.ActionMCPResolveReviewFinding, map[string]interface{}{
		"finding_id": id, "status": "resolved",
	})
	unreachableResp, err := h.handleResolveReviewFinding(context.Background(), unreachable)
	require.NoError(t, err)
	assertWSError(t, unreachableResp, ws.ErrorCodeNotFound)

	unknown := makeWSMessage(t, ws.ActionMCPResolveReviewFinding, map[string]interface{}{
		"finding_id": "does-not-exist", "status": "resolved",
	})
	unknownResp, err := h.handleResolveReviewFinding(context.Background(), unknown)
	require.NoError(t, err)
	assertWSError(t, unknownResp, ws.ErrorCodeNotFound)

	var unreachableErr, unknownErr ws.ErrorPayload
	require.NoError(t, json.Unmarshal(unreachableResp.Payload, &unreachableErr))
	require.NoError(t, json.Unmarshal(unknownResp.Payload, &unknownErr))
	require.Equal(t, unknownErr.Message, unreachableErr.Message)
}
