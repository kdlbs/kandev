package models

import (
	"strings"
	"testing"
	"time"
)

func TestTaskTurnErrorSanitizedUTF8Bounds(t *testing.T) {
	long := strings.Repeat("é", maxTurnErrorMessageBytes)
	value := NormalizeTaskTurnError(TaskTurnError{
		StampValue: strings.Repeat("s", 100),
		SessionID:  strings.Repeat("i", 400),
		OccurredAt: time.Now().UTC(),
		Message:    long,
	})
	if len(value.Message) > maxTurnErrorMessageBytes {
		t.Fatalf("message bytes = %d, want <= %d", len(value.Message), maxTurnErrorMessageBytes)
	}
	if !strings.HasPrefix(value.Message, "é") || strings.ContainsRune(value.Message, '\uFFFD') {
		t.Fatalf("message truncated on a rune boundary: %q", value.Message)
	}
	if len(value.StampValue) > maxTurnErrorStampBytes {
		t.Fatalf("stamp bytes = %d", len(value.StampValue))
	}
	if len(value.SessionID) > maxTurnErrorIDBytes {
		t.Fatalf("session bytes = %d", len(value.SessionID))
	}
	if !value.Valid() {
		t.Fatal("bounded record must stay valid")
	}
}

func TestSetAndAcknowledgeTaskTurnErrorPreserveNewerFailure(t *testing.T) {
	metadata := map[string]interface{}{}
	first := NormalizeTaskTurnError(TaskTurnError{
		StampValue: "error-1", SessionID: "session-a", OccurredAt: time.Unix(100, 0).UTC(),
		Message: "first failure",
	})
	if !SetTaskTurnError(metadata, first) {
		t.Fatal("first attention was not stored")
	}
	if SetTaskTurnError(metadata, first) {
		t.Fatal("duplicate stamp must not refresh occurrence time")
	}
	older := first
	older.StampValue = "error-0"
	older.OccurredAt = time.Unix(50, 0).UTC()
	if SetTaskTurnError(metadata, older) {
		t.Fatal("an older failure must not replace a newer one")
	}
	newer := NormalizeTaskTurnError(TaskTurnError{
		StampValue: "error-2", SessionID: "session-b", OccurredAt: time.Unix(200, 0).UTC(),
		Message: "newer failure",
	})
	if !SetTaskTurnError(metadata, newer) {
		t.Fatal("a newer failure must replace the pointer")
	}
	if AcknowledgeTaskTurnError(metadata, first.StampValue, time.Unix(300, 0), TurnErrorAcknowledgementPrompt) {
		t.Fatal("a stale acknowledgement must not retire the newer failure")
	}
	if !AcknowledgeTaskTurnError(metadata, newer.StampValue, time.Unix(300, 0), TurnErrorAcknowledgementDismiss) {
		t.Fatal("the current failure must be acknowledgeable")
	}
	current, ok := LoadTaskTurnError(metadata)
	if !ok || !current.Acknowledged() || current.AcknowledgedBy != TurnErrorAcknowledgementDismiss {
		t.Fatalf("current record = %+v, want a dismissed newer failure", current)
	}
	if !AcknowledgeTaskTurnError(metadata, newer.StampValue, time.Unix(400, 0), TurnErrorAcknowledgementDismiss) {
		t.Fatal("repeat dismissal must be idempotent")
	}
	current, _ = LoadTaskTurnError(metadata)
	if !current.AcknowledgedAt.Equal(time.Unix(300, 0).UTC()) {
		t.Fatalf("acknowledged_at = %v, want the first acknowledgement time", current.AcknowledgedAt)
	}
}
