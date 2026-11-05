package handlers

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestSessionDeleteErrorCodePreservesPendingQueueConflict(t *testing.T) {
	if got := sessionDeleteErrorCode(fmt.Errorf("delete session: %w", messagequeue.ErrSessionQueueNotEmpty)); got != ws.ErrorCodeValidation {
		t.Fatalf("pending queue code = %q, want validation", got)
	}
	if got := sessionDeleteErrorCode(errors.New("database unavailable")); got != ws.ErrorCodeInternalError {
		t.Fatalf("database failure code = %q, want internal", got)
	}
}

func TestSessionDeleteErrorMessageOmitsRepositoryDetails(t *testing.T) {
	for _, err := range []error{
		fmt.Errorf("session secret, 3 rows in queue table: %w", messagequeue.ErrSessionQueueNotEmpty),
		errors.New("private SQL statement"),
	} {
		if message := sessionDeleteErrorMessage(err); strings.Contains(message, "secret") || strings.Contains(message, "SQL") || strings.Contains(message, "queue table") {
			t.Fatalf("internal detail leaked: %q", message)
		}
	}
}
