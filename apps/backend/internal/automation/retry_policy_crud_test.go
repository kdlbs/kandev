package automation

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestCreateAutomationRequestCarriesRetryPolicy(t *testing.T) {
	var req CreateAutomationRequest
	if err := json.Unmarshal([]byte(`{"retry_policy":{"mode":"finite","max_retries":"2","delay_seconds":"30","backoff":"exponential","history_mode":"timeline"}}`), &req); err != nil {
		t.Fatalf("decode create request: %v", err)
	}
	field := reflect.ValueOf(req).FieldByName("RetryPolicy")
	if !field.IsValid() {
		t.Fatal("CreateAutomationRequest is missing RetryPolicy")
	}
	got, ok := field.Interface().(RetryPolicy)
	if !ok {
		t.Fatalf("RetryPolicy field type = %s, want automation.RetryPolicy", field.Type())
	}
	want := RetryPolicy{Mode: RetryModeFinite, MaxRetries: "2", DelaySeconds: "30", Backoff: RetryBackoffExponential, HistoryMode: RetryHistoryTimeline}
	if got != want {
		t.Fatalf("RetryPolicy = %#v, want %#v", got, want)
	}
}

func TestUpdateAutomationRequestCarriesOptionalRetryPolicy(t *testing.T) {
	var req UpdateAutomationRequest
	if err := json.Unmarshal([]byte(`{"retry_policy":{"mode":"infinite","delay_seconds":"0"}}`), &req); err != nil {
		t.Fatalf("decode update request: %v", err)
	}
	field := reflect.ValueOf(req).FieldByName("RetryPolicy")
	if !field.IsValid() {
		t.Fatal("UpdateAutomationRequest is missing RetryPolicy")
	}
	if field.Kind() != reflect.Pointer || field.Type().Elem() != reflect.TypeOf(RetryPolicy{}) {
		t.Fatalf("RetryPolicy field type = %s, want *automation.RetryPolicy", field.Type())
	}
	if field.IsNil() {
		t.Fatal("RetryPolicy was not decoded")
	}
}

func TestStorePersistsDisabledRetryPolicyByDefault(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-1", Name: "retry defaults"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatalf("create automation: %v", err)
	}

	var raw string
	if err := store.db.GetContext(ctx, &raw, store.db.Rebind(`SELECT retry_policy FROM automations WHERE id = ?`), a.ID); err != nil {
		t.Fatalf("read persisted retry policy: %v", err)
	}
	if raw != `{"mode":"disabled","max_retries":"0","delay_seconds":"0","backoff":"fixed","history_mode":"attempts"}` {
		t.Fatalf("persisted retry policy = %q", raw)
	}
	got, err := store.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatalf("reload automation: %v", err)
	}
	field := reflect.ValueOf(*got).FieldByName("RetryPolicy")
	if !field.IsValid() {
		t.Fatal("Automation is missing RetryPolicy")
	}
	if gotPolicy, ok := field.Interface().(RetryPolicy); !ok || gotPolicy != (RetryPolicy{Mode: RetryModeDisabled, MaxRetries: "0", DelaySeconds: "0", Backoff: RetryBackoffFixed, HistoryMode: RetryHistoryAttempts}) {
		t.Fatalf("reloaded retry policy = %#v", field.Interface())
	}
}
func TestServiceCreateAndUpdateRetryPolicyAtomically(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	created, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID: "ws-1",
		Name:        "retry policy",
		RetryPolicy: RetryPolicy{
			Mode:         RetryModeFinite,
			MaxRetries:   "2",
			DelaySeconds: "30",
			Backoff:      RetryBackoffExponential,
			HistoryMode:  RetryHistoryTimeline,
		},
	})
	if err != nil {
		t.Fatalf("create automation: %v", err)
	}
	want := RetryPolicy{
		Mode:         RetryModeFinite,
		MaxRetries:   "2",
		DelaySeconds: "30",
		Backoff:      RetryBackoffExponential,
		HistoryMode:  RetryHistoryTimeline,
	}
	if created.RetryPolicy != want {
		t.Fatalf("created retry policy = %#v, want %#v", created.RetryPolicy, want)
	}

	updatedPolicy := RetryPolicy{Mode: RetryModeInfinite, DelaySeconds: "5"}
	updated, err := svc.UpdateAutomation(ctx, created.ID, &UpdateAutomationRequest{RetryPolicy: &updatedPolicy})
	if err != nil {
		t.Fatalf("update retry policy: %v", err)
	}
	normalized := RetryPolicy{
		Mode:         RetryModeInfinite,
		MaxRetries:   "0",
		DelaySeconds: "5",
		Backoff:      RetryBackoffFixed,
		HistoryMode:  RetryHistoryAttempts,
	}
	if updated.RetryPolicy != normalized {
		t.Fatalf("updated retry policy = %#v, want %#v", updated.RetryPolicy, normalized)
	}

	name := "must not partially update"
	invalid := RetryPolicy{Mode: RetryModeFinite, MaxRetries: "0"}
	if _, err := svc.UpdateAutomation(ctx, created.ID, &UpdateAutomationRequest{Name: &name, RetryPolicy: &invalid}); err == nil {
		t.Fatal("invalid retry policy update succeeded")
	}
	reloaded, err := svc.GetAutomation(ctx, created.ID)
	if err != nil {
		t.Fatalf("reload automation after invalid update: %v", err)
	}
	if reloaded.Name != "retry policy" || reloaded.RetryPolicy != normalized {
		t.Fatalf("invalid update partially applied: name=%q policy=%#v", reloaded.Name, reloaded.RetryPolicy)
	}
}
