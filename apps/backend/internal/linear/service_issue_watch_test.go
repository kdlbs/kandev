package linear

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/kandev/kandev/internal/integrations/optional"
)

func TestService_CreateIssueWatch_AcceptsRichFilters(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	min := 1.0
	cases := map[string]SearchFilter{
		"priority":    {Priorities: []int{1}},
		"labels":      {LabelIDs: []string{"l1"}},
		"creator":     {CreatorID: "u1"},
		"estimateMin": {EstimateMin: &min},
	}
	for name, filter := range cases {
		w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
			WorkspaceID:    "ws-1",
			WorkflowID:     "wf",
			WorkflowStepID: "step",
			Filter:         filter,
		})
		if err != nil {
			t.Errorf("%s: create rejected: %v", name, err)
			continue
		}
		if w.ID == "" {
			t.Errorf("%s: expected ID assigned", name)
		}
	}
}

// withSearchResults returns a fakeClient that always returns the given issues
// for SearchIssues, ignoring the filter.
func (c *fakeClient) withSearchResults(issues []LinearIssue) *fakeClient {
	c.searchIssuesFn = func(_ SearchFilter, _ string, _ int) (*SearchResult, error) {
		return &SearchResult{Issues: issues, IsLast: true}, nil
	}
	return c
}

func TestService_CreateIssueWatch_DefaultsAndValidation(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	// Empty filter is rejected.
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for empty filter, got %v", err)
	}

	// Whitespace-only fields are also rejected (normalize then check).
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
		Filter:         SearchFilter{Query: "   ", TeamKey: " ", StateIDs: []string{" ", ""}},
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for whitespace-only filter, got %v", err)
	}

	// Happy path assigns ID + defaults Enabled=true.
	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
		Filter:         SearchFilter{TeamKey: "ENG"},
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
	if w.Filter.TeamKey != "ENG" {
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
		Filter:         SearchFilter{TeamKey: "ENG"},
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
	if updated.Filter.TeamKey != "ENG" || updated.WorkspaceID != created.WorkspaceID {
		t.Errorf("unexpected mutation of unset fields: %+v", updated)
	}

	// Patching filter to something empty is rejected to keep watch rows valid.
	empty := SearchFilter{}
	if _, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{Filter: &empty}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for empty filter patch, got %v", err)
	}
}

func TestService_IssueWatch_MaxInflightTasks(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	// Default create (no cap supplied) persists NULL → reads back as nil
	// (uncapped). The store column default of 5 must NOT leak through the
	// INSERT path, which names the column explicitly with a NULL bind.
	uncapped, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{TeamKey: "ENG"},
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

	// Create with a positive cap round-trips through the nullable column.
	cap5 := 5
	capped, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{TeamKey: "ENG"}, MaxInflightTasks: &cap5,
	})
	if err != nil {
		t.Fatalf("create capped: %v", err)
	}
	if capped.MaxInflightTasks == nil || *capped.MaxInflightTasks != 5 {
		t.Fatalf("cap not persisted: %v", capped.MaxInflightTasks)
	}

	// Non-positive caps are rejected on create.
	for _, bad := range []int{0, -1} {
		b := bad
		if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
			WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
			Filter: SearchFilter{TeamKey: "ENG"}, MaxInflightTasks: &b,
		}); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("expected ErrInvalidConfig for cap=%d, got %v", bad, err)
		}
	}

	// PATCH is tri-state: present+int sets the cap, present+null clears it,
	// absent leaves it unchanged. The absent case is the footgun greptile/the
	// local reviewer flagged — a partial update must NOT silently drop the cap.
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

	// Partial PATCH that omits MaxInflightTasks (Present=false) must preserve
	// the existing cap of 3 — not reset it to uncapped.
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

	// Explicit null clears the cap back to uncapped.
	cleared, err := f.svc.UpdateIssueWatch(ctx, capped.ID, &UpdateIssueWatchRequest{
		MaxInflightTasks: optional.Int{Present: true, Value: nil},
	})
	if err != nil {
		t.Fatalf("update clear cap: %v", err)
	}
	if cleared.MaxInflightTasks != nil {
		t.Fatalf("expected cap cleared to nil, got %v", *cleared.MaxInflightTasks)
	}

	// Non-positive cap is rejected on update too.
	zero := 0
	if _, err := f.svc.UpdateIssueWatch(ctx, capped.ID, &UpdateIssueWatchRequest{
		MaxInflightTasks: optional.Int{Present: true, Value: &zero},
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for update cap=0, got %v", err)
	}
}

func TestValidateSortBy(t *testing.T) {
	for _, ok := range []IssueSortBy{
		SortByDefault, SortByPriorityDesc, SortByPriorityAsc,
		SortByCreatedDesc, SortByCreatedAsc, SortByUpdatedDesc, SortByUpdatedAsc,
	} {
		if err := validateSortBy(ok); err != nil {
			t.Errorf("validateSortBy(%q) rejected a known value: %v", ok, err)
		}
	}
	if err := validateSortBy(IssueSortBy("bogus")); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for unknown sortBy, got %v", err)
	}
}

func TestService_IssueWatch_SortByRoundTrips(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	created, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{TeamKey: "ENG"}, SortBy: SortByPriorityDesc,
	})
	if err != nil {
		t.Fatalf("create with sortBy: %v", err)
	}
	if created.SortBy != SortByPriorityDesc {
		t.Fatalf("sortBy not set on create: %q", created.SortBy)
	}
	got, err := f.svc.GetIssueWatch(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SortBy != SortByPriorityDesc {
		t.Fatalf("sortBy did not round-trip: %q", got.SortBy)
	}

	// Create with an unknown sortBy is rejected.
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{TeamKey: "ENG"}, SortBy: IssueSortBy("bogus"),
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for unknown sortBy on create, got %v", err)
	}
}

func TestValidateFilterBounds_RejectsNonFiniteAndNegativeEstimates(t *testing.T) {
	nan := math.NaN()
	posInf := math.Inf(1)
	negInf := math.Inf(-1)
	neg := -1.0
	cases := map[string]SearchFilter{
		"estimateMin NaN":  {EstimateMin: &nan},
		"estimateMax NaN":  {EstimateMax: &nan},
		"estimateMin +Inf": {EstimateMin: &posInf},
		"estimateMax -Inf": {EstimateMax: &negInf},
		"estimateMin < 0":  {EstimateMin: &neg},
		"estimateMax < 0":  {EstimateMax: &neg},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validateFilterBounds(f); !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("expected ErrInvalidConfig, got %v", err)
			}
		})
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
			Filter:              SearchFilter{TeamKey: "ENG"},
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
		Filter:         SearchFilter{TeamKey: "ENG"},
	}); err != nil {
		t.Errorf("expected zero pollIntervalSeconds to be accepted, got %v", err)
	}
}

func TestService_UpdateIssueWatch_RejectsEmptyWorkflowFields(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{TeamKey: "ENG"},
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
		AuthMethod: AuthMethodAPIKey, Secret: "lin_api",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}

	f.client.withSearchResults([]LinearIssue{
		{Identifier: "ENG-1", Title: "one", URL: "https://linear.app/x/issue/ENG-1"},
		{Identifier: "ENG-2", Title: "two", URL: "https://linear.app/x/issue/ENG-2"},
		{Identifier: "ENG-3", Title: "three", URL: "https://linear.app/x/issue/ENG-3"},
	})

	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{TeamKey: "ENG"},
	})
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}

	// Pre-seed ENG-2 as already turned into a task.
	if _, err := f.store.ReserveIssueWatchTask(ctx, w.ID, "ENG-2", "https://linear.app/x/issue/ENG-2"); err != nil {
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
		if i.Identifier == "ENG-2" {
			t.Error("ENG-2 should have been filtered as already seen")
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
		AuthMethod: AuthMethodAPIKey, Secret: "lin_api",
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	f.client.searchIssuesFn = func(_ SearchFilter, _ string, _ int) (*SearchResult, error) {
		return nil, errors.New("upstream 500")
	}
	w, _ := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{TeamKey: "ENG"},
	})
	if _, err := f.svc.CheckIssueWatch(ctx, w); err == nil {
		t.Error("expected error from search to surface to caller")
	}
	refreshed, _ := f.store.GetIssueWatch(ctx, w.ID)
	if refreshed.LastPolledAt == nil {
		t.Error("expected last_polled_at stamped even on search failure (liveness signal)")
	}
}
