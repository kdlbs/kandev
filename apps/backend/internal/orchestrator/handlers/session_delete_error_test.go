package handlers

import (
	"errors"
	"fmt"
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
