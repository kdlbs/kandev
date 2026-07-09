package sentry

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/integrations/optional"
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
	instID := f.ensureInstance(t, "ws-1")

	// Bad filters are rejected before the instance check, so no instance needed.
	for name, filter := range map[string]SearchFilter{
		"empty":          {},
		"org only":       {OrgSlug: "acme"},
		"whitespace org": {OrgSlug: "   ", ProjectSlug: "frontend"},
		"multi status":   {OrgSlug: "acme", ProjectSlug: "frontend", Statuses: []string{"unresolved", "ignored"}},
	} {
		if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
			WorkspaceID: "ws-1", SentryInstanceID: instID,
			WorkflowID: "wf", WorkflowStepID: "step", Filter: filter,
		}); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s filter should be rejected, got %v", name, err)
		}
	}

	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", SentryInstanceID: instID,
		WorkflowID: "wf", WorkflowStepID: "step", Filter: validFilter(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if w.ID == "" || !w.Enabled {
		t.Fatalf("unexpected created watch: %+v", w)
	}
	if w.SentryInstanceID != instID {
		t.Errorf("expected instance %q bound, got %q", instID, w.SentryInstanceID)
	}
}

// TestService_CreateIssueWatch_InstanceValidation pins the required + owned
// contract on the bound instance.
func TestService_CreateIssueWatch_InstanceValidation(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	other := f.seedInstance(t, "ws-2", "Other", "")

	// Missing instance → ErrInvalidConfig.
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step", Filter: validFilter(),
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for missing instance, got %v", err)
	}
	// Instance from another workspace → ErrInstanceNotFound.
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", SentryInstanceID: other.ID,
		WorkflowID: "wf", WorkflowStepID: "step", Filter: validFilter(),
	}); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound for cross-workspace instance, got %v", err)
	}
}

func TestService_UpdateIssueWatch_LegacyMultiStatusToggle(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()

	// Seed a legacy watch (unbound, multi-status) directly through the store.
	w := newTestIssueWatch("ws-1")
	w.Filter.Statuses = []string{"unresolved", "ignored"}
	if err := f.store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("seed watch: %v", err)
	}
	disabled := false
	if _, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{Enabled: &disabled}); err != nil {
		t.Fatalf("toggle on legacy multi-status watch should succeed, got %v", err)
	}
	bad := SearchFilter{OrgSlug: "acme", ProjectSlug: "frontend", Statuses: []string{"unresolved", "ignored"}}
	if _, err := f.svc.UpdateIssueWatch(ctx, w.ID, &UpdateIssueWatchRequest{Filter: &bad}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("filter patch with multiple statuses should be rejected, got %v", err)
	}
}

func TestService_UpdateIssueWatch_PartialPatch(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", SentryInstanceID: f.ensureInstance(t, "ws-1"),
		WorkflowID: "wf", WorkflowStepID: "step", Filter: validFilter(), Prompt: "original",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	newPrompt := "updated"
	updated, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{Prompt: &newPrompt})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Prompt != "updated" || updated.Filter.OrgSlug != "acme" || updated.WorkspaceID != created.WorkspaceID {
		t.Errorf("unexpected mutation: %+v", updated)
	}
	empty := SearchFilter{}
	if _, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{Filter: &empty}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for empty filter patch, got %v", err)
	}
}

// TestService_UpdateIssueWatch_InstanceImmutable pins acceptance (g): the bound
// instance is absent from the update request, so it cannot be changed.
func TestService_UpdateIssueWatch_InstanceImmutable(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	instA := f.seedInstance(t, "ws-1", "A", "")
	created, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", SentryInstanceID: instA.ID,
		WorkflowID: "wf", WorkflowStepID: "step", Filter: validFilter(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	prompt := "changed"
	updated, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{Prompt: &prompt})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.SentryInstanceID != instA.ID {
		t.Errorf("instance changed on update: got %q want %q", updated.SentryInstanceID, instA.ID)
	}
	reloaded, _ := f.store.GetIssueWatch(ctx, created.ID)
	if reloaded.SentryInstanceID != instA.ID {
		t.Errorf("persisted instance changed: got %q want %q", reloaded.SentryInstanceID, instA.ID)
	}
}

func TestService_IssueWatch_MaxInflightTasks(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	instID := f.ensureInstance(t, "ws-1")
	base := func() *CreateIssueWatchRequest {
		return &CreateIssueWatchRequest{
			WorkspaceID: "ws-1", SentryInstanceID: instID,
			WorkflowID: "wf", WorkflowStepID: "step", Filter: validFilter(),
		}
	}

	req := base()
	req.MaxInflightTasks = new(0)
	if _, err := f.svc.CreateIssueWatch(ctx, req); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for non-positive cap, got %v", err)
	}
	req = base()
	req.MaxInflightTasks = new(3)
	created, err := f.svc.CreateIssueWatch(ctx, req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	reloaded, _ := f.svc.GetIssueWatch(ctx, created.ID)
	if reloaded.MaxInflightTasks == nil || *reloaded.MaxInflightTasks != 3 {
		t.Fatalf("expected cap 3 persisted, got %v", reloaded.MaxInflightTasks)
	}
	unchanged, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{})
	if err != nil || unchanged.MaxInflightTasks == nil || *unchanged.MaxInflightTasks != 3 {
		t.Errorf("omitted cap should stay 3, got %v (err %v)", unchanged.MaxInflightTasks, err)
	}
	uncapped, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{
		MaxInflightTasks: optional.Int{Present: true, Value: nil},
	})
	if err != nil || uncapped.MaxInflightTasks != nil {
		t.Errorf("null cap should clear to uncapped, got %v (err %v)", uncapped.MaxInflightTasks, err)
	}
	if _, err := f.svc.UpdateIssueWatch(ctx, created.ID, &UpdateIssueWatchRequest{
		MaxInflightTasks: optional.Int{Present: true, Value: new(-1)},
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for negative cap patch, got %v", err)
	}
}

func TestService_CreateIssueWatch_RejectsOutOfRangeInterval(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	instID := f.ensureInstance(t, "ws-1")
	for _, n := range []int{1, 30, 59, 3601, 86400} {
		if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
			WorkspaceID: "ws-1", SentryInstanceID: instID,
			WorkflowID: "wf", WorkflowStepID: "step", Filter: validFilter(), PollIntervalSeconds: n,
		}); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("expected ErrInvalidConfig for pollIntervalSeconds=%d, got %v", n, err)
		}
	}
	if _, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", SentryInstanceID: instID,
		WorkflowID: "wf", WorkflowStepID: "step", Filter: validFilter(),
	}); err != nil {
		t.Errorf("expected zero pollIntervalSeconds to be accepted, got %v", err)
	}
}

func TestService_UpdateIssueWatch_RejectsEmptyWorkflowFields(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	created, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", SentryInstanceID: f.ensureInstance(t, "ws-1"),
		WorkflowID: "wf", WorkflowStepID: "step", Filter: validFilter(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	empty := ""
	for _, req := range []*UpdateIssueWatchRequest{{WorkflowID: &empty}, {WorkflowStepID: &empty}} {
		if _, err := f.svc.UpdateIssueWatch(ctx, created.ID, req); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("expected ErrInvalidConfig for %+v, got %v", req, err)
		}
	}
}

func TestService_UpdateIssueWatch_NotFound(t *testing.T) {
	f := newSvcFixture(t)
	prompt := "x"
	if _, err := f.svc.UpdateIssueWatch(context.Background(), "ghost", &UpdateIssueWatchRequest{Prompt: &prompt}); !errors.Is(err, ErrIssueWatchNotFound) {
		t.Errorf("expected ErrIssueWatchNotFound, got %v", err)
	}
}

func TestService_CheckIssueWatch_FiltersAlreadySeen(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	inst := f.seedInstance(t, "ws-1", "A", "sntrys_abc")
	f.client.withSearchResults([]SentryIssue{
		{ShortID: "PROJ-1", Title: "one", Permalink: "https://sentry.io/issues/PROJ-1"},
		{ShortID: "PROJ-2", Title: "two", Permalink: "https://sentry.io/issues/PROJ-2"},
		{ShortID: "PROJ-3", Title: "three", Permalink: "https://sentry.io/issues/PROJ-3"},
	})
	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", SentryInstanceID: inst.ID,
		WorkflowID: "wf", WorkflowStepID: "step", Filter: validFilter(),
	})
	if err != nil {
		t.Fatalf("create watch: %v", err)
	}
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
	inst := f.seedInstance(t, "ws-1", "A", "sntrys_abc")
	f.client.searchIssuesFn = func(_ SearchFilter, _ string) (*SearchResult, error) {
		return nil, errors.New("upstream 500")
	}
	w, err := f.svc.CreateIssueWatch(ctx, &CreateIssueWatchRequest{
		WorkspaceID: "ws-1", SentryInstanceID: inst.ID,
		WorkflowID: "wf", WorkflowStepID: "step", Filter: validFilter(),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := f.svc.CheckIssueWatch(ctx, w); err == nil {
		t.Error("expected error from search to surface to caller")
	}
	refreshed, _ := f.store.GetIssueWatch(ctx, w.ID)
	if refreshed.LastPolledAt == nil {
		t.Error("expected last_polled_at stamped even on search failure (liveness signal)")
	}
}

// TestService_CheckIssueWatch_ResolvesSoleInstance pins acceptance (b): an
// unbound (NULL-instance) watch resolves to its workspace's sole instance at
// poll time.
func TestService_CheckIssueWatch_ResolvesSoleInstance(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	f.seedInstance(t, "ws-1", "Primary", "tok") // the sole instance, with a secret
	f.client.withSearchResults([]SentryIssue{
		{ShortID: "PROJ-1", Title: "one", Permalink: "https://sentry.io/issues/PROJ-1"},
	})
	// Watch created directly with a NULL instance (migrated legacy row).
	w := newTestIssueWatch("ws-1")
	if err := f.store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("seed unbound watch: %v", err)
	}
	if w.SentryInstanceID != "" {
		t.Fatalf("expected unbound watch, got instance %q", w.SentryInstanceID)
	}
	got, err := f.svc.CheckIssueWatch(ctx, w)
	if err != nil {
		t.Fatalf("check unbound watch: %v", err)
	}
	if len(got) != 1 || got[0].ShortID != "PROJ-1" {
		t.Fatalf("expected sole-instance resolution to return PROJ-1, got %+v", got)
	}
}

// TestService_CheckIssueWatch_UnboundNoInstanceStampsError pins the other half
// of acceptance (b): an unbound watch whose workspace has no instance stamps a
// last_error and skips, without disabling the watch.
func TestService_CheckIssueWatch_UnboundNoInstanceStampsError(t *testing.T) {
	f := newSvcFixture(t)
	ctx := context.Background()
	w := newTestIssueWatch("ws-empty") // workspace has zero instances
	if err := f.store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("seed watch: %v", err)
	}
	if _, err := f.svc.CheckIssueWatch(ctx, w); err == nil {
		t.Error("expected an error when no instance can be resolved")
	}
	refreshed, _ := f.store.GetIssueWatch(ctx, w.ID)
	if refreshed.LastError == "" {
		t.Error("expected last_error stamped when resolution fails")
	}
	if !refreshed.Enabled {
		t.Error("watch must stay enabled (stamp + skip, not disable) so it auto-heals")
	}
}
