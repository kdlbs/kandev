package clarification

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestPendingIDForRequest_IsStablePerSessionAndRetryKey(t *testing.T) {
	first := PendingIDForRequest("session-a", "conn-1/int64:7")
	require.NotEmpty(t, first, "expected a retry identity")
	assert.Equal(t, first, PendingIDForRequest("session-a", "conn-1/int64:7"), "exact retry must map to the same identity")
	assert.NotEqual(t, first, PendingIDForRequest("session-b", "conn-1/int64:7"), "identity must be scoped to the session")
	assert.NotEqual(t, first, PendingIDForRequest("session-a", "conn-2/int64:7"), "a restarted id on another connection must not alias")
	assert.NotEqual(t, first, PendingIDForRequest("session-a", "conn-1/int64:8"), "a different request on the same connection must not alias")
}

func TestPendingIDForRequest_EmptyInputsKeepRandomStoreIdentity(t *testing.T) {
	assert.Empty(t, PendingIDForRequest("session-a", ""))
	assert.Empty(t, PendingIDForRequest("", "conn-1/int64:7"))
}

func retryBundleMessage(id string, index int, status string, response map[string]any) *taskmodels.Message {
	meta := map[string]any{
		"pending_id":     "p-retry",
		"question_id":    "q" + string(rune('1'+index)),
		"question_index": index,
		"status":         status,
	}
	if response != nil {
		meta["response"] = response
	}
	return &taskmodels.Message{ID: id, TaskSessionID: "sess-1", Metadata: meta}
}

func TestRecordedOutcome_PendingBundleIsNotRecorded(t *testing.T) {
	msgs := []*taskmodels.Message{
		retryBundleMessage("m1", 0, "pending", nil),
		retryBundleMessage("m2", 1, "pending", nil),
	}
	_, resp, ok := RecordedOutcome("p-retry", msgs, nil)
	assert.False(t, ok)
	assert.Nil(t, resp)
}

func TestRecordedOutcome_NoMessagesIsNotRecorded(t *testing.T) {
	_, _, ok := RecordedOutcome("p-retry", nil, nil)
	assert.False(t, ok)
}

func TestRecordedOutcome_AnsweredBundleReturnsRecordedAnswers(t *testing.T) {
	msgs := []*taskmodels.Message{
		retryBundleMessage("m2", 1, "answered", map[string]any{"custom_text": "second"}),
		retryBundleMessage("m1", 0, "answered", map[string]any{"selected_options": []any{"opt-a"}}),
	}
	status, resp, ok := RecordedOutcome("p-retry", msgs, nil)
	require.True(t, ok)
	assert.Equal(t, StatusAnswered, status)
	require.NotNil(t, resp)
	assert.Equal(t, "p-retry", resp.PendingID)
	assert.False(t, resp.Rejected)
	require.Len(t, resp.Answers, 2)
	assert.Equal(t, "q1", resp.Answers[0].QuestionID)
	assert.Equal(t, []string{"opt-a"}, resp.Answers[0].SelectedOptions)
	assert.Equal(t, "q2", resp.Answers[1].QuestionID)
	assert.Equal(t, "second", resp.Answers[1].CustomText)
}

func TestRecordedOutcome_RejectedBundleReturnsRejection(t *testing.T) {
	msgs := []*taskmodels.Message{retryBundleMessage("m1", 0, "rejected", nil)}
	status, resp, ok := RecordedOutcome("p-retry", msgs, nil)
	require.True(t, ok)
	assert.Equal(t, StatusRejected, status)
	require.NotNil(t, resp)
	assert.True(t, resp.Rejected)
	assert.Empty(t, resp.Answers)
}

func TestRecordedOutcome_CancelledBundleReportsClosedWithoutResponse(t *testing.T) {
	msgs := []*taskmodels.Message{
		retryBundleMessage("m1", 0, "cancelled", nil),
		retryBundleMessage("m2", 1, "cancelled", nil),
	}
	status, resp, ok := RecordedOutcome("p-retry", msgs, nil)
	require.True(t, ok)
	assert.Equal(t, StatusCancelled, status)
	assert.Nil(t, resp)
}

func TestRecordedOutcome_ExpiredBundleReportsClosedWithoutResponse(t *testing.T) {
	msgs := []*taskmodels.Message{retryBundleMessage("m1", 0, "expired", nil)}
	status, resp, ok := RecordedOutcome("p-retry", msgs, nil)
	require.True(t, ok)
	assert.Equal(t, StatusExpired, status)
	assert.Nil(t, resp)
}

func TestRecordedOutcome_PartiallyCancelledBundleStaysAnswerable(t *testing.T) {
	msgs := []*taskmodels.Message{
		retryBundleMessage("m1", 0, "cancelled", nil),
		retryBundleMessage("m2", 1, "pending", nil),
	}
	_, _, ok := RecordedOutcome("p-retry", msgs, nil)
	assert.False(t, ok, "a bundle with an open question must still be waited on")
}
