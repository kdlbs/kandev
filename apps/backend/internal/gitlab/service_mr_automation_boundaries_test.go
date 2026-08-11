package gitlab

import (
	"errors"
	"testing"
)

func TestServiceMRAutomationOperationsRequireStore(t *testing.T) {
	service := NewService(DefaultHost, NewMockClient(DefaultHost), "mock", nil, newTestLogger(t))
	tests := []struct {
		name string
		call func() error
	}{
		{name: "enabled prompts", call: func() error { _, err := service.HasEnabledTaskMRAgentPrompts(t.Context(), "task"); return err }},
		{name: "rebind reviewer", call: func() error { _, _, err := service.RebindTaskMRReviewer(t.Context(), "task"); return err }},
		{name: "get lifecycle state", call: func() error {
			_, err := service.GetTaskMRLifecycleState(t.Context(), "task", "repo", "group/project", 1)
			return err
		}},
		{name: "review request", call: func() error {
			return service.SetTaskMRReviewRequestState(t.Context(), "task", "repo", "group/project", 1, true)
		}},
		{name: "observed state", call: func() error {
			return service.SetTaskMRObservedState(t.Context(), "task", "repo", "group/project", 1, "merged")
		}},
		{name: "lifecycle prompt", call: func() error {
			return service.RecordTaskMRLifecyclePrompt(t.Context(), TaskMRLifecyclePrompt{TaskID: "task"})
		}},
		{name: "automation error", call: func() error {
			return service.RecordTaskMRAutomationError(t.Context(), "task", "repo", "group/project", 1, "failed")
		}},
		{name: "clear automation error", call: func() error {
			return service.ClearTaskMRAutomationError(t.Context(), "task", "repo", "group/project", 1)
		}},
		{name: "sync error", call: func() error {
			return service.RecordTaskMRSyncError(t.Context(), "task", "repo", "group/project", 1, "failed")
		}},
		{name: "clear sync error", call: func() error { return service.ClearTaskMRSyncError(t.Context(), "task", "repo", "group/project", 1) }},
		{name: "fix attempt", call: func() error { return service.RecordTaskMRFixAttempt(t.Context(), TaskMRFixAttempt{TaskID: "task"}) }},
		{name: "refresh fix checkpoint", call: func() error {
			return service.RefreshTaskMRFixCheckpoint(t.Context(), "task", "repo", "group/project", 1, "sig", `{}`)
		}},
		{name: "fix exhausted", call: func() error {
			return service.MarkTaskMRAutoFixExhausted(t.Context(), "task", "repo", "group/project", 1, "limit")
		}},
		{name: "merge attempt", call: func() error { return service.RecordTaskMRMergeAttempt(t.Context(), TaskMRMergeAttempt{TaskID: "task"}) }},
		{name: "subscribed MRs", call: func() error { _, err := service.ListAutomationSubscribedTaskMRs(t.Context()); return err }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, errStoreUnavailable) {
				t.Fatalf("error = %v, want errStoreUnavailable", err)
			}
		})
	}
}

func TestServiceMRAutomationStateRoundTrip(t *testing.T) {
	store := newTestStore(t)
	seedWorkspace(t, store, "workspace")
	seedTask(t, store, "task", "workspace")
	service := NewService(DefaultHost, NewMockClient(DefaultHost), "mock", nil, newTestLogger(t))
	service.SetStore(store)

	if err := service.SetTaskMRReviewRequestState(t.Context(), "task", "repo", "group/project", 1, true); err != nil {
		t.Fatal(err)
	}
	if err := service.SetTaskMRObservedState(t.Context(), "task", "repo", "group/project", 1, "opened"); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordTaskMRAutomationError(t.Context(), "task", "repo", "group/project", 1, " delivery failed "); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordTaskMRSyncError(t.Context(), "task", "repo", "group/project", 1, " sync failed "); err != nil {
		t.Fatal(err)
	}
	state, err := service.GetTaskMRLifecycleState(t.Context(), "task", "repo", "group/project", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !state.LastReviewRequested || state.LastObservedState != "opened" || stringValue(state.LastError) != " delivery failed " || stringValue(state.LastSyncError) != " sync failed " {
		t.Fatalf("state = %#v", state)
	}

	if err := service.ClearTaskMRAutomationError(t.Context(), "task", "repo", "group/project", 1); err != nil {
		t.Fatal(err)
	}
	if err := service.ClearTaskMRSyncError(t.Context(), "task", "repo", "group/project", 1); err != nil {
		t.Fatal(err)
	}
	state, err = service.GetTaskMRLifecycleState(t.Context(), "task", "repo", "group/project", 1)
	if err != nil || state.LastError != nil || state.LastSyncError != nil {
		t.Fatalf("cleared state = (%#v, %v)", state, err)
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
