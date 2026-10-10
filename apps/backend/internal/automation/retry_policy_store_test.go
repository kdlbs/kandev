package automation

import (
	"context"
	"testing"
)

func TestAutomationRetryPolicyRoundTrip(t *testing.T) {
	store := setupTestStore(t)
	a := &Automation{
		WorkspaceID: "ws-1", Name: "Retrying", Enabled: true,
		RetryPolicy: RetryPolicy{
			Mode: RetryModeFinite, MaxRetries: "3", DelaySeconds: "60",
			Backoff: RetryBackoffExponential, HistoryMode: RetryHistoryTimeline,
		},
	}
	if err := store.CreateAutomation(context.Background(), a); err != nil {
		t.Fatalf("create automation: %v", err)
	}
	got, err := store.GetAutomation(context.Background(), a.ID)
	if err != nil {
		t.Fatalf("get automation: %v", err)
	}
	if got.RetryPolicy != a.RetryPolicy {
		t.Fatalf("retry policy mismatch: got %#v, want %#v", got.RetryPolicy, a.RetryPolicy)
	}
	if err := store.UpdateAutomation(context.Background(), a.ID, &UpdateAutomationRequest{
		RetryPolicy: &RetryPolicy{Mode: RetryModeInfinite, DelaySeconds: "0"},
	}); err != nil {
		t.Fatalf("update retry policy: %v", err)
	}
	got, err = store.GetAutomation(context.Background(), a.ID)
	if err != nil {
		t.Fatalf("get updated automation: %v", err)
	}
	want := RetryPolicy{Mode: RetryModeInfinite, MaxRetries: "0", DelaySeconds: "0", Backoff: RetryBackoffFixed, HistoryMode: RetryHistoryAttempts}
	if got.RetryPolicy != want {
		t.Fatalf("updated retry policy mismatch: got %#v, want %#v", got.RetryPolicy, want)
	}
}
