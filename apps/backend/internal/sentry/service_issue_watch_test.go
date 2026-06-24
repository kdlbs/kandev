package sentry

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/integrations/optional"
)

func intPtr(v int) *int { return &v }

// withSearchResults returns a fakeClient that always returns the given issues
// for SearchIssues, ignoring the filter.
func (c *fakeClient) withSearchResults(issues []SentryIssue) *fakeClient {
	c.searchIssuesFn = func(_ SearchFilter, _ string) (*SearchResult, error) {
		return &SearchResult{Issues: issues, IsLast: true}, nil
	}
	return c
}

func validFilter() SearchFilter {
	return SearchFilter{OrgSlug: "acme", ProjectSlug: "frontend"}
}

func TestService_CreateIssueWatch_DefaultsAndValidation(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	// Missing org/project is rejected — a watch must name what it polls.
	for name, filter := range map[string]SearchFilter{
		"empty":          {},
		"org only":       {OrgSlug: "acme"},
		"whitespace org": {OrgSlug: "   ", ProjectSlug: "frontend"},
		// Sentry has no OR form for is:, so multiple statuses would AND-combine
		// into a query that matches nothing — reject at save time.
		"multi status": {OrgSlug: "acme", ProjectSlug: "frontend", Statuses: []string{"unresolved", "ignored"}},
	} {
		if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
			WorkspaceID:    "ws-1",
			WorkflowID:     "wf",
			WorkflowStepID: "step",
			Filter:         filter,
		}); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s filter should be rejected, got %v", name, err)
		}
	}

	// Happy path assigns ID + defaults Enabled=true.
	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
		Filter:         validFilter(),
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
	if w.Filter.OrgSlug != "acme" || w.Filter.ProjectSlug != "frontend" {
		t.Errorf("filter not persisted: %+v", w.Filter)
	}
}

func TestService_UpdateIssueWatch_LegacyMultiStatusToggle(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	// Seed a legacy watch carrying multiple statuses directly through the
	// store, bypassing service validation — mirrors rows created before the
	// single-status rule existed.
	w := newTestIssueWatch("ws-1")
	w.Filter.Statuses = []string{"unresolved", "ignored"}
	if err := f.store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("seed watch: %v", err)
	}

	// A partial update that leaves the filter untouched (the enable/disable
	// toggle sends only {enabled}) must still succeed for the legacy watch.
	disabled := false
	if _, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{Enabled: &disabled}); err != nil {
		t.Fatalf("toggle on legacy multi-status watch should succeed, got %v", err)
	}

	// Patching the filter to keep multiple statuses must be rejected.
	bad := SearchFilter{OrgSlug: "acme", ProjectSlug: "frontend", Statuses: []string{"unresolved", "ignored"}}
	if _, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{Filter: &bad}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("filter patch with multiple statuses should be rejected, got %v", err)
	}
}

func TestService_UpdateIssueWatch_PartialPatch(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	created, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
		Filter:         validFilter(),
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
	if updated.Filter.OrgSlug != "acme" || updated.WorkspaceID != created.WorkspaceID {
		t.Errorf("unexpected mutation of unset fields: %+v", updated)
	}

	// Patching the filter to something empty is rejected to keep watch rows valid.
	empty := SearchFilter{}
	if _, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{Filter: &empty}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for empty filter patch, got %v", err)
	}
}

func TestService_IssueWatch_MaxInflightTasks(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	// A non-positive cap is rejected at create.
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:      "ws-1",
		WorkflowID:       "wf",
		WorkflowStepID:   "step",
		Filter:           validFilter(),
		MaxInflightTasks: intPtr(0),
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for non-positive cap, got %v", err)
	}

	// A positive cap round-trips through the store.
	created, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:      "ws-1",
		WorkflowID:       "wf",
		WorkflowStepID:   "step",
		Filter:           validFilter(),
		MaxInflightTasks: intPtr(3),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	reloaded, err := f.svc.GetIssueWatch(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if reloaded.MaxInflightTasks == nil || *reloaded.MaxInflightTasks != 3 {
		t.Fatalf("expected cap 3 persisted, got %v", reloaded.MaxInflightTasks)
	}

	// PATCH tri-state: omitted leaves the cap unchanged.
	unchanged, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{})
	if err != nil {
		t.Fatalf("patch (omit): %v", err)
	}
	if unchanged.MaxInflightTasks == nil || *unchanged.MaxInflightTasks != 3 {
		t.Errorf("omitted cap should stay 3, got %v", unchanged.MaxInflightTasks)
	}

	// PATCH tri-state: explicit null clears the cap to uncapped.
	uncapped, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{
		MaxInflightTasks: optional.Int{Present: true, Value: nil},
	})
	if err != nil {
		t.Fatalf("patch (null): %v", err)
	}
	if uncapped.MaxInflightTasks != nil {
		t.Errorf("null cap should clear to uncapped, got %v", *uncapped.MaxInflightTasks)
	}

	// PATCH tri-state: a non-positive value is rejected.
	if _, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{
		MaxInflightTasks: optional.Int{Present: true, Value: intPtr(-1)},
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for negative cap patch, got %v", err)
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
			Filter:              validFilter(),
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
		Filter:         validFilter(),
	}); err != nil {
		t.Errorf("expected zero pollIntervalSeconds to be accepted, got %v", err)
	}
}

func TestService_UpdateIssueWatch_RejectsEmptyWorkflowFields(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: validFilter(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	empty := ""
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

	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "sntrys_abc",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}

	f.client.withSearchResults([]SentryIssue{
		{ShortID: "PROJ-1", Title: "one", Permalink: "https://sentry.io/issues/PROJ-1"},
		{ShortID: "PROJ-2", Title: "two", Permalink: "https://sentry.io/issues/PROJ-2"},
		{ShortID: "PROJ-3", Title: "three", Permalink: "https://sentry.io/issues/PROJ-3"},
	})

	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: validFilter(),
	})
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}

	// Pre-seed PROJ-2 as already turned into a task.
	if _, err := f.store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-2", "https://sentry.io/issues/PROJ-2"); err != nil {
		t.Fatalf("seed reservation: %v", err)
	}

	got, err := f.svc.CheckIssueWatch(ctx, w)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 unseen issues, got %d", len(got))
	}
	for _, i := range got {
		if i.ShortID == "PROJ-2" {
			t.Error("PROJ-2 should have been filtered as already seen")
		}
	}

	refreshed, _ := f.store.GetIssueWatch(ctx, w.ID)
	if refreshed.LastPolledAt == nil {
		t.Error("expected last_polled_at stamped after check")
	}
}

func TestService_CheckIssueWatch_StampsLastPolledOnError(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	if _, err := f.svc.SetConfig(ctx, &SetConfigRequest{
		AuthMethod: AuthMethodAuthToken, Secret: "sntrys_abc",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	f.client.searchIssuesFn = func(_ SearchFilter, _ string) (*SearchResult, error) {
		return nil, errors.New("upstream 500")
	}
	w, _ := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: validFilter(),
	})
	if _, err := f.svc.CheckIssueWatch(ctx, w); err == nil {
		t.Error("expected error from search to surface to caller")
	}
	refreshed, _ := f.store.GetIssueWatch(ctx, w.ID)
	if refreshed.LastPolledAt == nil {
		t.Error("expected last_polled_at stamped even on search failure (liveness signal)")
	}
}
