package scope

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	userstore "github.com/kandev/kandev/internal/user/store"
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

// TestScopeUnownedWorkspaceStaysBounded covers a workspace with no owner —
// created before auth was enabled, or since by an internal (unscoped) caller.
// Such rows stay publicly visible by the compatibility contract, but the stream
// must NOT be handed an identity-free context: that reads as an internal caller
// and grants every user's data. It gets an ID no account can hold, so the
// service's "empty owner_id or my own ID" rule limits it to unowned rows.
func TestScopeUnownedWorkspaceStaysBounded(t *testing.T) {
	lookup := ownedFixture()
	lookup.workspaces["ws-1"].OwnerID = ""
	r := newResolver(t, lookup, fakeIdentityLookup{}, true)

	ctx, err := r.Scope(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}

	identity := identityOf(t, ctx)
	if identity.UserID != unownedScopeUserID {
		t.Errorf("UserID = %q, want the unowned sentinel %q", identity.UserID, unownedScopeUserID)
	}
	if identity.Synthetic {
		t.Error("the sentinel must not be synthetic — synthetic reads as unscoped")
	}
	if identity.Role == authn.RoleAdmin {
		t.Error("the sentinel must not carry the admin role")
	}
}

// TestScopeTaskWithoutWorkspaceStaysBounded is the same reasoning for a task
// that has no workspace at all.
func TestScopeTaskWithoutWorkspaceStaysBounded(t *testing.T) {
	lookup := ownedFixture()
	lookup.tasks["task-1"].WorkspaceID = ""
	r := newResolver(t, lookup, fakeIdentityLookup{}, true)

	ctx, err := r.Scope(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("Scope: %v", err)
	}

	if got := identityOf(t, ctx).UserID; got != unownedScopeUserID {
		t.Errorf("UserID = %q, want the unowned sentinel %q", got, unownedScopeUserID)
	}
}

// TestUnownedScopeUserIDCannotBeAStoredAccount pins the sentinel's one
// requirement: it must not collide with a real user ID (UUIDs, or the
// default-user row promoted by the setup wizard).
func TestUnownedScopeUserIDCannotBeAStoredAccount(t *testing.T) {
	if unownedScopeUserID == "" {
		t.Fatal("the sentinel must be non-empty, or callerScope reports unscoped")
	}
	if !strings.Contains(unownedScopeUserID, ":") {
		t.Errorf("sentinel %q should keep a colon so it cannot be a UUID", unownedScopeUserID)
	}
	if _, err := uuid.Parse(unownedScopeUserID); err == nil {
		t.Errorf("sentinel %q parses as a UUID and could collide with a real account", unownedScopeUserID)
	}
	if unownedScopeUserID == userstore.DefaultUserID {
		t.Error("sentinel collides with the default-user row")
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

// TestScopeDeniesWhenOwnerAccountInactive covers a deleted or disabled owner.
// Disabling a user revokes their sessions and PATs, so a still-running agent
// session would otherwise be the one remaining way into their workspace —
// deny the dispatch instead of minting an identity on their behalf.
func TestScopeDeniesWhenOwnerAccountInactive(t *testing.T) {
	r := newResolver(t, ownedFixture(), fakeIdentityLookup{}, true)

	if _, err := r.Scope(context.Background(), "task-1"); err == nil {
		t.Fatal("expected an error so the dispatch is denied for an inactive owner")
	}
}

// TestScopeFailsClosedOnMissingTaskRow and its workspace sibling cover a
// lookup that reports "not found" as (nil, nil) rather than an error. An
// unscoped context here would let a stream whose task was deleted mid-session
// read everything.
func TestScopeFailsClosedOnMissingTaskRow(t *testing.T) {
	lookup := &fakeTaskOwnerLookup{
		tasks:      map[string]*models.Task{},
		workspaces: map[string]*models.Workspace{},
	}
	r := newResolver(t, lookup, fakeIdentityLookup{}, true)

	if _, err := r.Scope(context.Background(), "task-gone"); err == nil {
		t.Fatal("expected an error for a missing task row")
	}
}

func TestScopeFailsClosedOnMissingWorkspaceRow(t *testing.T) {
	lookup := ownedFixture()
	delete(lookup.workspaces, "ws-1")
	r := newResolver(t, lookup, fakeIdentityLookup{}, true)

	if _, err := r.Scope(context.Background(), "task-1"); err == nil {
		t.Fatal("expected an error for a missing workspace row")
	}
}

// TestScopeFailsClosedWithoutIdentityLookup guards the wiring: a resolver with
// no identity lookup must deny, not fall back to unscoped.
func TestScopeFailsClosedWithoutIdentityLookup(t *testing.T) {
	r := NewResolver(ownedFixture(), nil, func() bool { return true }, testLogger(t))

	if _, err := r.Scope(context.Background(), "task-1"); err == nil {
		t.Fatal("expected an error when no identity lookup is wired")
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
