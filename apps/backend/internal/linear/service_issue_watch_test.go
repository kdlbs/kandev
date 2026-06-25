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

// fakeRepoLookup is a static RepositoryLookup for repository-binding tests.
type fakeRepoLookup struct {
	workspaceID   string
	defaultBranch string
	ok            bool
}

func (f fakeRepoLookup) GetRepository(_ context.Context, _ string) (string, string, bool) {
	return f.workspaceID, f.defaultBranch, f.ok
}

func TestService_CreateIssueWatch_RepositoryBinding(t *testing.T) {
	ctx := context.Background()
	baseReq := func() *CreateIssueWatchRequest {
		return &CreateIssueWatchRequest{
			WorkspaceID:    "ws-1",
			WorkflowID:     "wf",
			WorkflowStepID: "step",
			Filter:         SearchFilter{TeamKey: "ENG"},
		}
	}

	t.Run("default branch filled from repo", func(t *testing.T) {
		f := newSvcFixture(t)
		f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-1", defaultBranch: "main", ok: true})
		req := baseReq()
		req.RepositoryID = "repo-1"
		w, err := f.svc.CreateIssueWatch(ctx, req)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if w.RepositoryID != "repo-1" || w.BaseBranch != "main" {
			t.Fatalf("expected repo-1@main, got repo=%q branch=%q", w.RepositoryID, w.BaseBranch)
		}
	})

	t.Run("explicit branch preserved", func(t *testing.T) {
		f := newSvcFixture(t)
		f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-1", defaultBranch: "main", ok: true})
		req := baseReq()
		req.RepositoryID = "repo-1"
		req.BaseBranch = "release/v2"
		w, err := f.svc.CreateIssueWatch(ctx, req)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if w.BaseBranch != "release/v2" {
			t.Fatalf("explicit branch overwritten: %q", w.BaseBranch)
		}
	})

	t.Run("cross-workspace repo rejected", func(t *testing.T) {
		f := newSvcFixture(t)
		f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-other", defaultBranch: "main", ok: true})
		req := baseReq()
		req.RepositoryID = "repo-1"
		if _, err := f.svc.CreateIssueWatch(ctx, req); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("expected ErrInvalidConfig for cross-workspace repo, got %v", err)
		}
	})

	t.Run("missing repo rejected", func(t *testing.T) {
		f := newSvcFixture(t)
		f.svc.SetRepositoryLookup(fakeRepoLookup{ok: false})
		req := baseReq()
		req.RepositoryID = "ghost"
		if _, err := f.svc.CreateIssueWatch(ctx, req); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("expected ErrInvalidConfig for missing repo, got %v", err)
		}
	})

	t.Run("unbound skips lookup and clears branch", func(t *testing.T) {
		f := newSvcFixture(t) // no RepositoryLookup wired
		req := baseReq()
		req.BaseBranch = "ignored-without-repo"
		w, err := f.svc.CreateIssueWatch(ctx, req)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if w.RepositoryID != "" || w.BaseBranch != "" {
			t.Fatalf("unbound watch must stay empty, got repo=%q branch=%q", w.RepositoryID, w.BaseBranch)
		}
	})
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

func TestService_UpdateIssueWatch_RepositoryBinding(t *testing.T) {
	ctx := context.Background()
	f := newSvcFixture(t)
	f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-1", defaultBranch: "main", ok: true})

	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf",
		WorkflowStepID: "step",
		Filter:         SearchFilter{TeamKey: "ENG"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.RepositoryID != "" {
		t.Fatalf("expected unbound on create, got %q", w.RepositoryID)
	}

	// Bind an existing watch via update — the edit path must persist it.
	repoID := "repo-1"
	updated, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{RepositoryID: &repoID})
	if err != nil {
		t.Fatalf("update bind: %v", err)
	}
	if updated.RepositoryID != "repo-1" || updated.BaseBranch != "main" {
		t.Fatalf("expected repo-1@main after update, got repo=%q branch=%q", updated.RepositoryID, updated.BaseBranch)
	}

	// Unbind via update (empty string).
	empty := ""
	cleared, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{RepositoryID: &empty})
	if err != nil {
		t.Fatalf("update unbind: %v", err)
	}
	if cleared.RepositoryID != "" || cleared.BaseBranch != "" {
		t.Fatalf("expected cleared after update, got repo=%q branch=%q", cleared.RepositoryID, cleared.BaseBranch)
	}
}

func TestService_UpdateIssueWatch_RebindAndDeletedRepo(t *testing.T) {
	ctx := context.Background()
	f := newSvcFixture(t)
	f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-1", defaultBranch: "main", ok: true})

	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		Filter: SearchFilter{TeamKey: "ENG"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	repo1 := "repo-1"
	if _, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{RepositoryID: &repo1}); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// Rebind to a different repo with no explicit branch → branch resets to the
	// new repo's default ("develop"), not the previous repo's "main".
	f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-1", defaultBranch: "develop", ok: true})
	repo2 := "repo-2"
	rebound, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{RepositoryID: &repo2})
	if err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if rebound.RepositoryID != "repo-2" || rebound.BaseBranch != "develop" {
		t.Fatalf("rebind should reset branch to new default, got repo=%q branch=%q", rebound.RepositoryID, rebound.BaseBranch)
	}

	// Bound repo is now soft-deleted (lookup fails). Editing an unrelated field
	// must still succeed — the binding isn't part of this PATCH.
	f.svc.SetRepositoryLookup(fakeRepoLookup{ok: false})
	prompt := "updated prompt"
	edited, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{Prompt: &prompt})
	if err != nil {
		t.Fatalf("unrelated edit blocked by deleted bound repo: %v", err)
	}
	if edited.Prompt != "updated prompt" || edited.RepositoryID != "repo-2" || edited.BaseBranch != "develop" {
		t.Fatalf("expected prompt updated + binding preserved, got prompt=%q repo=%q branch=%q", edited.Prompt, edited.RepositoryID, edited.BaseBranch)
	}
}

func TestService_IssueWatch_RejectsInvalidBaseBranch(t *testing.T) {
	ctx := context.Background()
	f := newSvcFixture(t)
	f.svc.SetRepositoryLookup(fakeRepoLookup{workspaceID: "ws-1", defaultBranch: "main", ok: true})
	base := func() *CreateIssueWatchRequest {
		return &CreateIssueWatchRequest{
			WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
			Filter: SearchFilter{TeamKey: "ENG"}, RepositoryID: "repo-1",
		}
	}

	bad := base()
	bad.BaseBranch = "bad..ref"
	if _, err := f.svc.CreateIssueWatch(ctx, bad); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for invalid base branch on create, got %v", err)
	}

	w, err := f.svc.CreateIssueWatch(ctx, base()) // valid: empty branch -> default
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	badRef := "bad..ref"
	if _, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{BaseBranch: &badRef}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for invalid base branch on update, got %v", err)
	}
}
