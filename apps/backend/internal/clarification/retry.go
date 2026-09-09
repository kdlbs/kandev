package clarification

import (
	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// PendingIDForRequest derives the durable identity of one ask_user_question
// call from the Kandev session and the transport retry key the MCP server
// attached to the call. The retry key must already be scoped to one MCP
// connection so that JSON-RPC ids restarting on a new connection cannot alias
// an earlier bundle; binding the session as well keeps the identity from
// crossing sessions even if two connections reuse the same key. An empty
// session or retry key returns empty so callers keep the random-ID behavior.
func PendingIDForRequest(sessionID, retryKey string) string {
	if sessionID == "" || retryKey == "" {
		return ""
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("kandev/clarification/"+sessionID+"/"+retryKey)).String()
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
