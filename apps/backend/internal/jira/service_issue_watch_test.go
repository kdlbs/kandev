package jira

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/integrations/optional"
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

func TestService_IssueWatch_MaxInflightTasks(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	// Default create (no cap) persists NULL → reads back nil (uncapped); the
	// column default of 5 must not leak through the explicit-NULL INSERT.
	uncapped, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		JQL: "project = PROJ",
	})
	if err != nil {
		t.Fatalf("create uncapped: %v", err)
	}
	if uncapped.MaxInflightTasks != nil {
		t.Fatalf("expected nil (uncapped), got %v", *uncapped.MaxInflightTasks)
	}
	got, err := f.svc.GetIssueWatch(ctx, uncapped.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.MaxInflightTasks != nil {
		t.Fatalf("uncapped did not round-trip as nil: %v", *got.MaxInflightTasks)
	}

	// Positive cap round-trips.
	cap5 := 5
	capped, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		JQL: "project = PROJ", MaxInflightTasks: &cap5,
	})
	if err != nil {
		t.Fatalf("create capped: %v", err)
	}
	if capped.MaxInflightTasks == nil || *capped.MaxInflightTasks != 5 {
		t.Fatalf("cap not persisted: %v", capped.MaxInflightTasks)
	}

	// Non-positive caps rejected on create.
	for _, bad := range []int{0, -1} {
		b := bad
		if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
			WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
			JQL: "project = PROJ", MaxInflightTasks: &b,
		}); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("expected ErrInvalidConfig for cap=%d, got %v", bad, err)
		}
	}

	// PATCH is tri-state: present+int sets, present+null clears, absent leaves
	// the cap unchanged. The absent case must NOT silently drop the cap.
	cap3 := 3
	updated, err := f.svc.UpdateIssueWatch(ctx, capped.ID, &UpdateIssueWatchRequest{
		MaxInflightTasks: optional.Int{Present: true, Value: &cap3},
	})
	if err != nil {
		t.Fatalf("update set cap: %v", err)
	}
	if updated.MaxInflightTasks == nil || *updated.MaxInflightTasks != 3 {
		t.Fatalf("cap not updated: %v", updated.MaxInflightTasks)
	}

	// Partial PATCH that omits MaxInflightTasks preserves the existing cap.
	newPrompt := "changed"
	preserved, err := f.svc.UpdateIssueWatch(ctx, capped.ID, &UpdateIssueWatchRequest{
		Prompt: &newPrompt,
	})
	if err != nil {
		t.Fatalf("partial update: %v", err)
	}
	if preserved.MaxInflightTasks == nil || *preserved.MaxInflightTasks != 3 {
		t.Fatalf("partial PATCH wrongly cleared the cap: %v", preserved.MaxInflightTasks)
	}

	// Explicit null clears back to uncapped.
	cleared, err := f.svc.UpdateIssueWatch(ctx, capped.ID, &UpdateIssueWatchRequest{
		MaxInflightTasks: optional.Int{Present: true, Value: nil},
	})
	if err != nil {
		t.Fatalf("update clear cap: %v", err)
	}
	if cleared.MaxInflightTasks != nil {
		t.Fatalf("expected cap cleared to nil, got %v", *cleared.MaxInflightTasks)
	}

	// Non-positive cap rejected on update.
	zero := 0
	if _, err := f.svc.UpdateIssueWatch(ctx, capped.ID, &UpdateIssueWatchRequest{
		MaxInflightTasks: optional.Int{Present: true, Value: &zero},
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for update cap=0, got %v", err)
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
	if _, err := f.svc.SetConfigForWorkspace(ctx, "ws-1", &SetConfigRequest{
		SiteURL: "https://a.net", Email: "e",
		AuthMethod: AuthMethodAPIToken, InstanceType: InstanceTypeCloud, Secret: "tok",
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
		AuthMethod: AuthMethodAPIToken, InstanceType: InstanceTypeCloud, Secret: "tok",
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
