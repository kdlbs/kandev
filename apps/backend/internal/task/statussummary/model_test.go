package statussummary

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTaskStatusSummarySemanticEqualityIgnoresTransportMetadata(t *testing.T) {
	first := TaskStatusSummary{
		Revision:  1,
		UpdatedAt: time.Now().UTC(),
		PrimarySession: &PrimarySessionSummary{
			ID: "session-1", State: "WAITING_FOR_INPUT",
		},
		ActiveError: &ActiveErrorSummary{SessionID: "session-1", Stamp: "error-1", Preview: "failed"},
	}
	second := first
	second.Revision = 9
	second.UpdatedAt = first.UpdatedAt.Add(time.Hour)
	if !first.SemanticEqual(second) {
		t.Fatal("revision and updated_at should not affect semantic equality")
	}

	second.PrimarySession = &PrimarySessionSummary{ID: "session-1", State: "RUNNING"}
	if first.SemanticEqual(second) {
		t.Fatal("observed status changes must not compare equal")
	}
}

func TestTaskStatusSummaryLastActivityIsSemanticAndBackwardCompatible(t *testing.T) {
	activity := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	first := TaskStatusSummary{LastActivityAt: &activity}
	second := first
	if first.SemanticEqual(second) == false {
		t.Fatal("equal last activity timestamps should compare equal")
	}
	later := activity.Add(time.Minute)
	second.LastActivityAt = &later
	if first.SemanticEqual(second) {
		t.Fatal("different last activity timestamps must not compare equal")
	}
	payload, err := first.SemanticJSON()
	if err != nil {
		t.Fatalf("last activity semantic JSON: %v", err)
	}
	if !strings.Contains(string(payload), `"last_activity_at"`) {
		t.Fatalf("last activity missing from semantic JSON: %s", payload)
	}
	var decoded TaskStatusSummary
	if err := json.Unmarshal([]byte(`{"revision": 4, "updated_at": "2026-08-17T10:00:00Z"}`), &decoded); err != nil {
		t.Fatalf("decode legacy summary: %v", err)
	}
	if decoded.LastActivityAt != nil {
		t.Fatalf("legacy summary activity = %v, want nil", decoded.LastActivityAt)
	}
}

func TestTaskStatusSummarySemanticJSONIsBoundedAndOmitsTransportMetadata(t *testing.T) {
	summary := TaskStatusSummary{
		Revision:  4,
		UpdatedAt: time.Now().UTC(),
		ActiveError: &ActiveErrorSummary{
			SessionID: "session-1",
			Stamp:     "error-1",
			Preview:   "safe preview",
		},
		Git: &GitSummary{ChangedFiles: 2, Additions: 3},
	}
	payload, err := summary.SemanticJSON()
	if err != nil {
		t.Fatalf("semantic JSON: %v", err)
	}
	if strings.Contains(string(payload), `"revision"`) || strings.Contains(string(payload), `"updated_at"`) {
		t.Fatalf("semantic payload contains transport metadata: %s", payload)
	}
	var decoded TaskStatusSummary
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode semantic JSON: %v", err)
	}
	if !summary.SemanticEqual(decoded) {
		t.Fatalf("semantic round trip changed value: %#v", decoded)
	}
}

func TestTaskStatusSummaryQueuedPromptCountRoundTripsThroughSemanticJSON(t *testing.T) {
	summary := TaskStatusSummary{QueuedPromptCount: 4}
	payload, err := summary.SemanticJSON()
	if err != nil {
		t.Fatalf("semantic JSON: %v", err)
	}
	var decoded TaskStatusSummary
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode semantic JSON: %v", err)
	}
	if decoded.QueuedPromptCount != 4 {
		t.Fatalf("queued prompt count after round trip = %d, want 4", decoded.QueuedPromptCount)
	}
	if !summary.SemanticEqual(decoded) {
		t.Fatalf("semantic round trip changed value: %#v", decoded)
	}
}

func TestTaskStatusSummaryQueuedPromptCountValidateRejectsNegative(t *testing.T) {
	if err := (TaskStatusSummary{QueuedPromptCount: -1}).Validate(); err == nil {
		t.Fatal("negative queued prompt count should be rejected")
	}
	if err := (TaskStatusSummary{QueuedPromptCount: 0}).Validate(); err != nil {
		t.Fatalf("zero queued prompt count rejected: %v", err)
	}
}

func TestTaskStatusSummaryQueuedPromptCountAffectsSemanticEquality(t *testing.T) {
	base := TaskStatusSummary{}
	if base.SemanticEqual(TaskStatusSummary{QueuedPromptCount: 2}) {
		t.Fatal("different queued prompt counts must not compare equal")
	}
}

func TestTaskStatusSummaryValidateBoundsErrorPreview(t *testing.T) {
	valid := TaskStatusSummary{ActiveError: &ActiveErrorSummary{Preview: strings.Repeat("é", MaxActiveErrorPreviewBytes/2)}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid UTF-8 preview rejected: %v", err)
	}

	tooLarge := valid
	tooLarge.ActiveError = &ActiveErrorSummary{Preview: strings.Repeat("x", MaxActiveErrorPreviewBytes+1)}
	if err := tooLarge.Validate(); err == nil {
		t.Fatal("oversized preview should be rejected")
	}

	invalidUTF8 := TaskStatusSummary{ActiveError: &ActiveErrorSummary{Preview: string([]byte{0xff})}}
	if err := invalidUTF8.Validate(); err == nil {
		t.Fatal("invalid UTF-8 preview should be rejected")
	}
}

func TestTaskStatusSummaryProjectsTaskOwnedErrorFields(t *testing.T) {
	now := time.Date(2026, 8, 19, 20, 0, 0, 0, time.UTC)
	got := BuildFromAuthoritative(RebuildInput{
		TaskError: &ActiveErrorSummary{
			TaskRepositoryID: "task-repository-1",
			Stamp:            "task-error-1",
			OccurredAt:       now,
			Preview:          "The linked pull request is already closed or merged.",
			Category:         "pr_already_closed",
			RecoveryActions: []string{
				"mark_review_done", "unknown", "mark_review_done", "retry_default",
			},
		},
		Sessions: []RebuildSession{{
			ID: "older-session",
			ActiveError: &ActiveErrorSummary{
				SessionID:  "older-session",
				Stamp:      "session-error-1",
				OccurredAt: now.Add(-time.Minute),
				Preview:    "older",
			},
		}},
		Now: now,
	})

	if got.ActiveError == nil {
		t.Fatal("active error is nil")
	}
	if got.ActiveError.SessionID != "" || got.ActiveError.TaskRepositoryID != "task-repository-1" {
		t.Fatalf("active error identity = %+v", got.ActiveError)
	}
	if got.ActiveError.Category != "pr_already_closed" {
		t.Fatalf("active error category = %q", got.ActiveError.Category)
	}
	wantActions := []string{"mark_review_done", "retry_default"}
	if !reflect.DeepEqual(got.ActiveError.RecoveryActions, wantActions) {
		t.Fatalf("active error actions = %#v, want %#v", got.ActiveError.RecoveryActions, wantActions)
	}
}

func TestTaskStatusSummaryValidatesActiveErrorCategoryAndTaskRepositoryBounds(t *testing.T) {
	tooLargeCategory := TaskStatusSummary{ActiveError: &ActiveErrorSummary{
		Category: strings.Repeat("x", 65),
	}}
	if err := tooLargeCategory.Validate(); err == nil {
		t.Fatal("oversized active error category should be rejected")
	}
	tooLargeRepository := TaskStatusSummary{ActiveError: &ActiveErrorSummary{
		TaskRepositoryID: strings.Repeat("x", 257),
	}}
	if err := tooLargeRepository.Validate(); err == nil {
		t.Fatal("oversized task repository identity should be rejected")
	}
}

func TestBuildFromAuthoritativeUsesStableStampAsErrorTieBreaker(t *testing.T) {
	now := time.Date(2026, 8, 19, 20, 0, 0, 0, time.UTC)
	got := BuildFromAuthoritative(RebuildInput{
		Sessions: []RebuildSession{
			{ID: "session-a", ActiveError: &ActiveErrorSummary{
				SessionID: "session-a", Stamp: "stamp-a", OccurredAt: now, Preview: "a",
			}},
			{ID: "session-b", ActiveError: &ActiveErrorSummary{
				SessionID: "session-b", Stamp: "stamp-b", OccurredAt: now, Preview: "b",
			}},
		},
		Now: now,
	})
	if got.ActiveError == nil || got.ActiveError.Stamp != "stamp-b" {
		t.Fatalf("active error = %+v, want the stable-stamp winner", got.ActiveError)
	}
}
