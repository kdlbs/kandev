package jira

import (
	"context"
	"errors"
	"testing"
)

// withSearchResults returns a fakeClient that always returns the given tickets
// for SearchTickets, ignoring the JQL.
func (c *fakeClient) withSearchResults(tickets []JiraTicket) *fakeClient {
	c.searchFn = func(_ string) (*SearchResult, error) {
		return &SearchResult{Tickets: tickets, IsLast: true}, nil
	}
	return c
}

func TestService_CreateIssueWatch_DefaultsAndValidation(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	// Missing JQL is rejected.
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for missing JQL, got %v", err)
	}

	// Happy path assigns ID + defaults Enabled=true.
	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
		JQL:            "project = PROJ",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.ID == "" {
		t.Fatal("expected ID assigned")
	}
	if !w.Enabled {
		t.Error("expected Enabled defaulted to true")
	}
}

func TestService_UpdateIssueWatch_PartialPatch(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	created, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
		JQL:            "project = PROJ",
		Prompt:         "original",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Patch only the prompt; everything else must remain.
	newPrompt := "updated"
	updated, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{
		Prompt: &newPrompt,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Prompt != "updated" {
		t.Errorf("prompt not patched: %q", updated.Prompt)
	}
	if updated.JQL != created.JQL || updated.WorkspaceID != created.WorkspaceID {
		t.Errorf("unexpected mutation of unset fields: %+v", updated)
	}

	// Patching JQL to empty is rejected to keep watch rows valid.
	empty := "   "
	if _, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{JQL: &empty}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for empty JQL, got %v", err)
	}
}

func TestService_CreateIssueWatch_RejectsOutOfRangeInterval(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	for _, n := range []int{1, 30, 59, 3601, 86400} {
		_, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
			WorkspaceID:         "ws-1",
			WorkflowID:          "wf",
			WorkflowStepID:      "step",
			JQL:                 "project = PROJ",
			PollIntervalSeconds: n,
		})
		if !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("expected ErrInvalidConfig for pollIntervalSeconds=%d, got %v", n, err)
		}
	}
	// Zero is allowed (the store coerces to default).
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
		JQL:            "project = PROJ",
	}); err != nil {
		t.Errorf("expected zero pollIntervalSeconds to be accepted (default fill), got %v", err)
	}
}

func TestService_UpdateIssueWatch_RejectsEmptyWorkflowFields(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		JQL: "project = PROJ",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	empty := ""
	// `{"workflowId": ""}` would land here — the nil guard in applyIssueWatchPatch
	// doesn't catch explicit empty strings, and a watch with empty WorkflowID
	// would silently drop every event the orchestrator tries to handle.
	for _, req := range []*UpdateIssueWatchRequest{
		{WorkflowID: &empty},
		{WorkflowStepID: &empty},
	} {
		if _, err := f.svc.UpdateIssueWatch(ctx, created.ID, req); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("expected ErrInvalidConfig for %+v, got %v", req, err)
		}
	}
}

func TestService_UpdateIssueWatch_NotFound(t *testing.T) {
	f := newSvcFixture(t)
	prompt := "x"
	_, err := f.svc.UpdateIssueWatch(context.Background(), "ghost", &UpdateIssueWatchRequest{Prompt: &prompt})
	if !errors.Is(err, ErrIssueWatchNotFound) {
		t.Errorf("expected ErrIssueWatchNotFound, got %v", err)
	}
}

func TestService_CheckIssueWatch_FiltersAlreadySeen(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	// Configure JIRA so clientFor succeeds.
	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, Secret: "tok",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}

	// Search returns three tickets; one of them is already in the dedup table.
	f.client.withSearchResults([]JiraTicket{
		{Key: "PROJ-1", Summary: "one", URL: "https://a.net/browse/PROJ-1"},
		{Key: "PROJ-2", Summary: "two", URL: "https://a.net/browse/PROJ-2"},
		{Key: "PROJ-3", Summary: "three", URL: "https://a.net/browse/PROJ-3"},
	})

	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		JQL: "project = PROJ",
	})
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}

	// Pre-seed PROJ-2 as already turned into a task.
	if _, err := f.store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-2", "https://a.net/browse/PROJ-2"); err != nil {
		t.Fatalf("seed reservation: %v", err)
	}

	got, err := f.svc.CheckIssueWatch(ctx, w)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 unseen tickets, got %d", len(got))
	}
	for _, tk := range got {
		if tk.Key == "PROJ-2" {
			t.Error("PROJ-2 should have been filtered as already seen")
		}
	}

	// last_polled_at must have been stamped.
	refreshed, _ := f.store.GetIssueWatch(ctx, w.ID)
	if refreshed.LastPolledAt == nil {
		t.Error("expected last_polled_at stamped after check")
	}
}

func TestService_CheckIssueWatch_StampsLastPolledOnError(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, Secret: "tok",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	f.client.searchFn = func(_ string) (*SearchResult, error) {
		return nil, errors.New("upstream 500")
	}
	w, _ := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		JQL: "project = PROJ",
	})
	if _, err := f.svc.CheckIssueWatch(ctx, w); err == nil {
		t.Error("expected error from search to surface to caller")
	}
	refreshed, _ := f.store.GetIssueWatch(ctx, w.ID)
	if refreshed.LastPolledAt == nil {
		t.Error("expected last_polled_at stamped even on search failure (liveness signal)")
	}
}
