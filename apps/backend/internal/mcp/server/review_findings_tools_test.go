package mcp

// Coverage for the list_review_findings_kandev / resolve_review_finding_kandev
// tool handlers (REQ-TWS-003 / REQ-TWS-004): argument forwarding, the
// AC-TWS-003.13 bare-JSON text result, and error propagation.

import (
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListReviewFindingsTool_ForwardsTaskIDAndFilters(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"findings": []interface{}{}, "total_matched": float64(0), "truncated": false,
	}}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "list_review_findings_kandev", map[string]interface{}{
		"status":   "resolved",
		"severity": "blocker",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPListReviewFindings, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["task_id"], "omitted task_id must default to the session task")
	assert.Equal(t, "resolved", payload["status"])
	assert.Equal(t, "blocker", payload["severity"])
}

func TestListReviewFindingsTool_ExplicitTaskIDForwardedNotBound(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"findings": []interface{}{}, "total_matched": float64(0), "truncated": false,
	}}
	s := newTaskModeServer(t, backend, "task-A")

	result := callTool(t, s, "list_review_findings_kandev", map[string]interface{}{"task_id": "task-B"})

	assert.False(t, result.IsError)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-B", payload["task_id"], "explicit task_id must reach the backend, not the bound task")
}

// TestListReviewFindingsTool_SuccessResultIsBareJSON pins AC-TWS-003.13: the
// successful text result is the JSON document alone, no leading or trailing
// prose (a departure from publish_review_findings_kandev's "Review findings
// published:\n" prefix).
func TestListReviewFindingsTool_SuccessResultIsBareJSON(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"findings":      []interface{}{},
		"total_matched": float64(0),
		"truncated":     false,
	}}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "list_review_findings_kandev", map[string]interface{}{})

	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text.Text), &decoded),
		"the whole text result must parse as JSON with no prefix stripped")
	assert.Contains(t, decoded, "findings")
	assert.Contains(t, decoded, "total_matched")
	assert.Contains(t, decoded, "truncated")
}

// TestListReviewFindingsTool_NoTaskIDAndNoSessionTaskFails pins AC-TWS-003.12:
// with no explicit task_id and no session-bound task, the call fails the same
// "task_id is required" way the publisher does, rather than dispatching with an
// empty task_id.
func TestListReviewFindingsTool_NoTaskIDAndNoSessionTaskFails(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "")

	result := callTool(t, s, "list_review_findings_kandev", map[string]interface{}{})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction, "an unresolved task_id must never reach the backend")
}

func TestListReviewFindingsTool_BackendErrorSurfaces(t *testing.T) {
	backend := &testBackend{err: assertError("distinctive backend failure")}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "list_review_findings_kandev", map[string]interface{}{})

	assert.True(t, result.IsError)
}

func TestResolveReviewFindingTool_RequiresFindingIDAndStatus(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")

	missingFindingID := callTool(t, s, "resolve_review_finding_kandev", map[string]interface{}{"status": "resolved"})
	assert.True(t, missingFindingID.IsError)

	missingStatus := callTool(t, s, "resolve_review_finding_kandev", map[string]interface{}{"finding_id": "f-1"})
	assert.True(t, missingStatus.IsError)
	missingStatusText, ok := missingStatus.Content[0].(mcplib.TextContent)
	require.True(t, ok, "expected TextContent")
	assert.Contains(t, missingStatusText.Text, "open")
	assert.Contains(t, missingStatusText.Text, "resolved")
	assert.Contains(t, missingStatusText.Text, "dismissed")
}

func TestResolveReviewFindingTool_ForwardsArgsAndEnvelope(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"finding": map[string]interface{}{"id": "f-1", "status": "resolved", "resolved_at": nil},
	}}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "resolve_review_finding_kandev", map[string]interface{}{
		"finding_id": "f-1",
		"status":     "resolved",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPResolveReviewFinding, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "f-1", payload["finding_id"])
	assert.Equal(t, "resolved", payload["status"])

	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text.Text), &decoded),
		"the whole text result must parse as JSON with no prefix stripped")
	findingRaw, ok := decoded["finding"]
	require.True(t, ok, "response must carry a top-level finding key (AC-TWS-004.11)")
	finding, ok := findingRaw.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "resolved", finding["status"])
}

// TestResolveReviewFindingTool_NormalizesBeforeSchemaValidation pins
// AC-TWS-004.8: normalisation precedes schema validation, so a
// whitespace-padded, differently-cased status and finding_id are accepted
// rather than rejected by the schema layer's enum check.
func TestResolveReviewFindingTool_NormalizesBeforeSchemaValidation(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"finding": map[string]interface{}{"id": "f-1", "status": "resolved", "resolved_at": nil},
	}}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "resolve_review_finding_kandev", map[string]interface{}{
		"finding_id": " f-1 ",
		"status":     " RESOLVED ",
	})

	assert.False(t, result.IsError)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "f-1", payload["finding_id"], "finding_id must be trimmed before dispatch")
	assert.Equal(t, "resolved", payload["status"], "status must be trimmed and lower-cased before dispatch")
}

// TestResolveReviewFindingTool_UnrecognizedStatusNamesAcceptedValues pins
// AC-TWS-004.8/.1: a non-empty unrecognised status is rejected with a
// message naming all three accepted values, not the schema layer's generic
// enum error, and never reaches the backend.
func TestResolveReviewFindingTool_UnrecognizedStatusNamesAcceptedValues(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "resolve_review_finding_kandev", map[string]interface{}{
		"finding_id": "f-1",
		"status":     "bogus",
	})

	assert.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "open")
	assert.Contains(t, text.Text, "resolved")
	assert.Contains(t, text.Text, "dismissed")
	assert.Empty(t, backend.lastAction, "an invalid status must never reach the backend")
}

// TestResolveReviewFindingTool_WhitespaceFindingIDRejected pins AC-TWS-004.8:
// a finding_id empty after trimming is rejected before any write.
func TestResolveReviewFindingTool_WhitespaceFindingIDRejected(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "resolve_review_finding_kandev", map[string]interface{}{
		"finding_id": "   ",
		"status":     "resolved",
	})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction, "an empty finding_id must never reach the backend")
}

// TestResolveReviewFindingTool_SchemaRequiresFindingIDAndStatus pins
// AC-TWS-004.1: the tool's declared schema names both finding_id and status
// as required, matching AC-TWS-004.8's rejection of a missing status.
func TestResolveReviewFindingTool_SchemaRequiresFindingIDAndStatus(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")
	toolsMap := s.mcpServer.ListTools()
	st, ok := toolsMap["resolve_review_finding_kandev"]
	require.True(t, ok)

	schema, err := json.Marshal(st.Tool.InputSchema)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(schema, &parsed))

	required, _ := parsed["required"].([]interface{})
	assert.Contains(t, required, "finding_id")
	assert.Contains(t, required, "status")
}

func TestResolveReviewFindingTool_SchemaRestrictsStatusEnum(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")
	props := toolInputProperties(t, s, "resolve_review_finding_kandev")

	statusSchema, ok := props["status"].(map[string]interface{})
	require.True(t, ok)
	enumRaw, ok := statusSchema["enum"].([]interface{})
	require.True(t, ok, "status must declare an enum")

	enum := make([]string, 0, len(enumRaw))
	for _, v := range enumRaw {
		enum = append(enum, v.(string))
	}
	assert.ElementsMatch(t, []string{"open", "resolved", "dismissed"}, enum)
}

func TestResolveReviewFindingTool_BackendErrorSurfaces(t *testing.T) {
	backend := &testBackend{err: assertError("review finding not found")}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "resolve_review_finding_kandev", map[string]interface{}{
		"finding_id": "missing",
		"status":     "resolved",
	})

	assert.True(t, result.IsError)
}

// assertError is a plain error for testBackend fixtures.
type assertError string

func (e assertError) Error() string { return string(e) }
