package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestHandleStepComplete_TruncatesOversizedHandoff covers AC-003.1/003.2/003.4:
// an oversized handoff never strands the step (the call is still accepted and
// stored bounded), and the result reports which argument was truncated.
func TestHandleStepComplete_TruncatesOversizedHandoff(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-trunc", "session-trunc", "step-1", models.TaskSessionStateRunning)
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	oversized := strings.Repeat("h", stepCompletionSignalFieldLimitBytes+500)
	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-trunc",
		"session_id": "session-trunc",
		"summary":    "done",
		"handoff":    oversized,
	})
	resp, err := h.handleStepComplete(context.Background(), msg)
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, true, payload["accepted"], "an oversized value must never strand the step")
	truncatedRaw, ok := payload["truncated"].([]interface{})
	require.True(t, ok, "expected truncated array in response")
	assert.Equal(t, []interface{}{"handoff"}, truncatedRaw)
	assert.Equal(t, float64(stepCompletionSignalFieldLimitBytes), payload["truncation_limit_bytes"])

	session, err := repo.GetTaskSession(context.Background(), "session-trunc")
	require.NoError(t, err)
	bag, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok)
	assert.LessOrEqual(t, len(bag.Handoff), stepCompletionSignalFieldLimitBytes)
	assert.True(t, strings.HasSuffix(bag.Handoff, stepCompletionSignalTruncationMarker))
}

// TestHandleStepComplete_WhitespacePaddedHandoffTrimsBeforeMeasuring covers
// AC-003.2's trim-first ordering: a handoff that is oversized BEFORE
// trimming but at or under the ceiling AFTER trimming must be stored whole,
// with no truncation reported.
func TestHandleStepComplete_WhitespacePaddedHandoffTrimsBeforeMeasuring(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-trim-first", "session-trim-first", "step-1", models.TaskSessionStateRunning)
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	atLimit := strings.Repeat("h", stepCompletionSignalFieldLimitBytes)
	padded := strings.Repeat(" ", 500) + atLimit + strings.Repeat("\n", 500)
	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-trim-first",
		"session_id": "session-trim-first",
		"summary":    "done",
		"handoff":    padded,
	})
	resp, err := h.handleStepComplete(context.Background(), msg)
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, true, payload["accepted"])
	_, hasTruncated := payload["truncated"]
	assert.False(t, hasTruncated, "trimmed value is exactly at the ceiling and must not report truncation")

	session, err := repo.GetTaskSession(context.Background(), "session-trim-first")
	require.NoError(t, err)
	bag, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok)
	assert.Equal(t, atLimit, bag.Handoff, "stored value must equal the trimmed input exactly, whole and unmarked")
}

// TestHandleStepComplete_TruncatesBothFieldsInOrder covers AC-003.5: when
// both handoff and blockers truncate, the array lists "handoff" before
// "blockers".
func TestHandleStepComplete_TruncatesBothFieldsInOrder(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-trunc2", "session-trunc2", "step-1", models.TaskSessionStateRunning)
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	oversized := strings.Repeat("x", stepCompletionSignalFieldLimitBytes+1)
	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-trunc2",
		"session_id": "session-trunc2",
		"summary":    "done",
		"handoff":    oversized,
		"blockers":   oversized,
	})
	resp, err := h.handleStepComplete(context.Background(), msg)
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, []interface{}{"handoff", "blockers"}, payload["truncated"])
}

// TestHandleStepComplete_NoTruncationOmitsFields covers AC-003.5's converse:
// when nothing was truncated, both fields are entirely absent from the
// response rather than present with empty/zero values.
func TestHandleStepComplete_NoTruncationOmitsFields(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-notrunc", "session-notrunc", "step-1", models.TaskSessionStateRunning)
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-notrunc",
		"session_id": "session-notrunc",
		"summary":    "done",
		"handoff":    "short",
	})
	resp, err := h.handleStepComplete(context.Background(), msg)
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	_, hasTruncated := payload["truncated"]
	_, hasLimit := payload["truncation_limit_bytes"]
	assert.False(t, hasTruncated)
	assert.False(t, hasLimit)
}

// TestHandleStepComplete_DuplicateReportsNoTruncation covers AC-003.5a: a
// call rejected as a duplicate keeps its existing rejection shape and never
// reports truncation, even though its (discarded) values would have
// truncated.
func TestHandleStepComplete_DuplicateReportsNoTruncation(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-dup-trunc", "session-dup-trunc", "step-1", models.TaskSessionStateRunning)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-dup-trunc", models.SessionMetaKeyPendingStepCompletion, models.PendingStepCompletionSignal{
		StepID:     "step-1",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "first call",
		SignaledAt: time.Now().UTC(),
	}))
	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)

	oversized := strings.Repeat("y", stepCompletionSignalFieldLimitBytes+1)
	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-dup-trunc",
		"session_id": "session-dup-trunc",
		"summary":    "second call",
		"handoff":    oversized,
	})
	resp, err := h.handleStepComplete(ctx, msg)
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, false, payload["accepted"])
	assert.Equal(t, "already_signaled", payload["reason"])
	_, hasTruncated := payload["truncated"]
	assert.False(t, hasTruncated, "a rejected duplicate must report no truncation")
}
