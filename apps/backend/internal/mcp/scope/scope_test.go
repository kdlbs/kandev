package scope

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

type fakeTaskOwnerLookup struct {
	tasks      map[string]*models.Task
	workspaces map[string]*models.Workspace
	taskErr    error
	wsErr      error
}

func (f *fakeTaskOwnerLookup) GetTask(_ context.Context, id string) (*models.Task, error) {
	if f.taskErr != nil {
		return nil, f.taskErr
	}
	return f.tasks[id], nil
}

func (f *fakeTaskOwnerLookup) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	if f.wsErr != nil {
		return nil, f.wsErr
	}
	return f.workspaces[id], nil
}

type fakeIdentityLookup map[string]authn.Identity

func (f fakeIdentityLookup) IdentityForUser(_ context.Context, userID string) (authn.Identity, bool) {
	identity, ok := f[userID]
	return identity, ok
}

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// ownedFixture is one task in one workspace owned by "user-a".
func ownedFixture() *fakeTaskOwnerLookup {
	return &fakeTaskOwnerLookup{
		tasks:      map[string]*models.Task{"task-1": {ID: "task-1", WorkspaceID: "ws-1"}},
		workspaces: map[string]*models.Workspace{"ws-1": {ID: "ws-1", OwnerID: "user-a"}},
	}
}

func newResolver(t *testing.T, tasks TaskOwnerLookup, identities IdentityLookup, enforced bool) *Resolver {
	t.Helper()
	return NewResolver(tasks, identities, func() bool { return enforced }, testLogger(t))
}

func identityOf(t *testing.T, ctx context.Context) authn.Identity {
	t.Helper()
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		t.Fatal("expected an identity on the scoped context")
	}
	return identity
}

func TestScopeAttachesRealOwnerIdentity(t *testing.T) {
	identities := fakeIdentityLookup{"user-a": {UserID: "user-a", Role: authn.RoleAdmin}}
	r := newResolver(t, ownedFixture(), identities, true)

	ctx, err := r.Scope(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}

	identity := identityOf(t, ctx)
	if identity.UserID != "user-a" {
		t.Errorf("UserID = %q, want user-a", identity.UserID)
	}
	if identity.Role != authn.RoleAdmin {
		t.Errorf("Role = %q, want the owner's stored role (admin)", identity.Role)
	}
	if identity.Synthetic {
		t.Error("identity must be real, not synthetic — synthetic reads as unscoped")
	}
}

// TestScopeNoOpWhenAuthDisabled pins the opt-out path: single-user instances
// keep the pre-auth unscoped dispatch.
func TestScopeNoOpWhenAuthDisabled(t *testing.T) {
	r := newResolver(t, ownedFixture(), fakeIdentityLookup{}, false)

	ctx, err := r.Scope(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	if _, ok := authn.IdentityFromContext(ctx); ok {
		t.Error("auth disabled must leave the context unscoped")
	}
}

// TestScopeNoOpForUnownedWorkspace matches the task service's visibility rule:
// rows with an empty owner_id, created before auth was enabled, stay visible
// to everyone until the setup wizard claims them.
func TestScopeNoOpForUnownedWorkspace(t *testing.T) {
	lookup := ownedFixture()
	lookup.workspaces["ws-1"].OwnerID = ""
	r := newResolver(t, lookup, fakeIdentityLookup{}, true)

	ctx, err := r.Scope(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	if _, ok := authn.IdentityFromContext(ctx); ok {
		t.Error("an unclaimed workspace must stay unscoped")
	}
}

func TestScopeNoOpForTaskWithoutWorkspace(t *testing.T) {
	lookup := ownedFixture()
	lookup.tasks["task-1"].WorkspaceID = ""
	r := newResolver(t, lookup, fakeIdentityLookup{}, true)

	ctx, err := r.Scope(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	if _, ok := authn.IdentityFromContext(ctx); ok {
		t.Error("a task with no workspace has no owner to scope to")
	}
}

func TestScopeNoOpForEmptyTaskID(t *testing.T) {
	r := newResolver(t, ownedFixture(), fakeIdentityLookup{}, true)

	ctx, err := r.Scope(context.Background(), "")
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	if _, ok := authn.IdentityFromContext(ctx); ok {
		t.Error("no task ID means no owner to resolve")
	}
}

// TestScopePreservesExistingIdentity guards the external /mcp path and any
// other credentialed caller: an identity already on the context is never
// replaced or widened.
func TestScopePreservesExistingIdentity(t *testing.T) {
	identities := fakeIdentityLookup{"user-a": {UserID: "user-a", Role: authn.RoleAdmin}}
	r := newResolver(t, ownedFixture(), identities, true)
	caller := authn.Identity{UserID: "user-b", Role: authn.RoleMember, TokenID: "pat-1"}

	ctx, err := r.Scope(authn.WithIdentity(context.Background(), caller), "task-1")
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}

	if got := identityOf(t, ctx); got != caller {
		t.Errorf("identity = %+v, want the original caller %+v", got, caller)
	}
}

// TestScopeFailsClosedOnTaskLookupError is the fail-closed pin: if we cannot
// tell who owns the stream, denying the dispatch is correct — returning an
// unscoped context would grant every user's data.
func TestScopeFailsClosedOnTaskLookupError(t *testing.T) {
	lookup := ownedFixture()
	lookup.taskErr = errors.New("db unavailable")
	r := newResolver(t, lookup, fakeIdentityLookup{}, true)

	if _, err := r.Scope(context.Background(), "task-1"); err == nil {
		t.Fatal("expected an error so the dispatch is denied")
	}
}

func TestScopeFailsClosedOnWorkspaceLookupError(t *testing.T) {
	lookup := ownedFixture()
	lookup.wsErr = errors.New("db unavailable")
	r := newResolver(t, lookup, fakeIdentityLookup{}, true)

	if _, err := r.Scope(context.Background(), "task-1"); err == nil {
		t.Fatal("expected an error so the dispatch is denied")
	}
}

// TestScopeUsesLeastPrivilegeWhenOwnerAccountMissing covers a deleted or
// disabled owner: still scope to their user ID (so foreign workspaces stay
// hidden) but with the lowest role.
func TestScopeUsesLeastPrivilegeWhenOwnerAccountMissing(t *testing.T) {
	r := newResolver(t, ownedFixture(), fakeIdentityLookup{}, true)

	ctx, err := r.Scope(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}

	identity := identityOf(t, ctx)
	if identity.UserID != "user-a" {
		t.Errorf("UserID = %q, want the owner ID so scoping still applies", identity.UserID)
	}
	if identity.Role != authn.RoleMember {
		t.Errorf("Role = %q, want member", identity.Role)
	}
}

// TestScopeNoOpWithoutEnforcedCallback keeps a partially wired resolver from
// scoping (and therefore from denying) anything.
func TestScopeNoOpWithoutEnforcedCallback(t *testing.T) {
	r := NewResolver(ownedFixture(), fakeIdentityLookup{}, nil, testLogger(t))

	ctx, err := r.Scope(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}
	if _, ok := authn.IdentityFromContext(ctx); ok {
		t.Error("a resolver with no enforcement check must stay a no-op")
	}
}
