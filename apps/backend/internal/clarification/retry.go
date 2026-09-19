package clarification

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// PendingIDForRequest derives the durable identity of one ask_user_question
// call from the Kandev session, the transport retry key, and the immutable
// request payload. The retry key must already be scoped to one MCP connection
// so JSON-RPC ids restarting on a new connection cannot alias an earlier
// bundle. Including normalized questions and context also prevents a client
// that later reuses a completed JSON-RPC id on that same connection from
// adopting the old bundle. An empty session or retry key returns empty so
// callers keep the random-ID behavior.
func PendingIDForRequest(sessionID, retryKey string, questions []Question, context string) string {
	if sessionID == "" || retryKey == "" {
		return ""
	}
	payload, _ := json.Marshal(struct {
		Questions []Question `json:"questions"`
		Context   string     `json:"context"`
	}{Questions: questions, Context: context})
	return uuid.NewSHA1(uuid.NameSpaceURL, append([]byte("kandev/clarification/"+sessionID+"/"+retryKey+"/"), payload...)).String()
}

// RecordedOutcome reports the terminal state a bundle's durable messages
// already carry, so an exact retry of an interrupted ask_user_question call
// can return that outcome instead of waiting on a question nobody can answer
// anymore. ok is false while the bundle is still answerable. For an answered
// or rejected bundle response carries the recorded answers; for a bundle whose
// every question was cancelled or expired response is nil and status names
// that terminal state.
func RecordedOutcome(pendingID string, msgs []*taskmodels.Message, log *logger.Logger) (Status, *Response, bool) {
	if len(msgs) == 0 {
		return "", nil, false
	}
	if status, response, hasWinner := reconstructWinnerResolution(pendingID, msgs, log); hasWinner {
		return Status(status), response, true
	}
	closed := ""
	for _, m := range msgs {
		switch status := effectiveMessageStatus(m); Status(status) {
		case StatusCancelled, StatusExpired:
			if closed == "" {
				closed = status
			}
		default:
			return "", nil, false
		}
	}
	return Status(closed), nil, true
}
