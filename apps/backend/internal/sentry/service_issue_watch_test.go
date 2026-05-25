package sentry

import (
	"context"
	"errors"
	"testing"
)

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

	// Missing org/project is rejected.
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for empty filter, got %v", err)
	}

	// orgSlug only (no project) is rejected.
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
		Filter:         SearchFilter{OrgSlug: "acme"},
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for missing projectSlug, got %v", err)
	}

	// Whitespace-only org is treated as empty after normalize.
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
		Filter:         SearchFilter{OrgSlug: "   ", ProjectSlug: "frontend"},
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for whitespace-only orgSlug, got %v", err)
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

	// Patching filter to something empty is rejected to keep watch rows valid.
	empty := SearchFilter{}
	if _, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{Filter: &empty}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for empty filter patch, got %v", err)
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
