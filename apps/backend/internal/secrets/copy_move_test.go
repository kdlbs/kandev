package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
)

// testCopyMoveService builds a service over the user-visible SQLite store with
// an authorizer and existence checker that allow the listed workspaces.
func testCopyMoveService(t *testing.T, allowed map[string]bool) *Service {
	t.Helper()
	store := newTestSQLiteStore(t)
	svc := NewService(NewUserVisibleStore(store), nil)
	svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		if allowed[workspaceID] {
			return nil
		}
		return ErrWorkspaceAccessDenied
	})
	svc.SetWorkspaceExistenceChecker(func(_ context.Context, workspaceID string) error {
		if allowed[workspaceID] {
			return nil
		}
		return ErrWorkspaceAccessDenied
	})
	return svc
}

// decodeCopyRequest unmarshals a JSON copy/move request body for testing.
func decodeCopyRequest(t *testing.T, body string) *CopySecretRequest {
	t.Helper()
	var req CopySecretRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return &req
}

// TestServiceCopy_GlobalToWorkspaceUsesSourceNameWhenOmitted verifies a global-to-workspace copy with no explicit name keeps the source name and value and retains the source.
func TestServiceCopy_GlobalToWorkspaceUsesSourceNameWhenOmitted(t *testing.T) {
	svc := testCopyMoveService(t, map[string]bool{"workspace-a": true})
	ctx := context.Background()
	if _, err := svc.Create(ctx, &CreateSecretRequest{Name: "API Key", Value: "v1"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	item, err := svc.Copy(ctx, globalID(t, svc, "API Key"), "", decodeCopyRequest(t, `{"target_scope":"workspace","target_workspace_id":"workspace-a"}`))
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if item.Scope != ScopeWorkspace || item.WorkspaceID != "workspace-a" || item.Name != "API Key" {
		t.Fatalf("item = %+v", item)
	}
	if item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
		t.Fatalf("item timestamps zero: %+v", item)
	}
	value, err := svc.RevealForWorkspace(ctx, item.ID, "workspace-a")
	if err != nil {
		t.Fatalf("reveal: %v", err)
	}
	if value != "v1" {
		t.Fatalf("value = %q, want v1", value)
	}
	// Source retained.
	global, err := svc.List(ctx)
	if err != nil || len(global) != 1 {
		t.Fatalf("global list after copy = %+v, %v", global, err)
	}
}

// TestServiceCopy_WorkspaceToGlobalWithExplicitName verifies a workspace-to-global copy with an explicit name keeps the source intact.
func TestServiceCopy_WorkspaceToGlobalWithExplicitName(t *testing.T) {
	svc := testCopyMoveService(t, map[string]bool{"workspace-a": true})
	ctx := context.Background()
	w := mustCreateViaService(t, svc, "ws", "v2", ScopeWorkspace, "workspace-a")

	item, err := svc.Copy(ctx, w.ID, "workspace-a", decodeCopyRequest(t, `{"target_scope":"global","name":"renamed"}`))
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if item.Scope != ScopeGlobal || item.Name != "renamed" {
		t.Fatalf("item = %+v", item)
	}
	if _, err := svc.GetWorkspaceSecret(ctx, w.ID, "workspace-a"); err != nil {
		t.Fatalf("source lost after copy: %v", err)
	}
}

// TestServiceCopy_WorkspaceToWorkspace verifies a workspace-to-workspace copy lands in the target workspace.
func TestServiceCopy_WorkspaceToWorkspace(t *testing.T) {
	svc := testCopyMoveService(t, map[string]bool{"workspace-a": true, "workspace-b": true})
	ctx := context.Background()
	w := mustCreateViaService(t, svc, "ws", "v3", ScopeWorkspace, "workspace-a")

	item, err := svc.Copy(ctx, w.ID, "workspace-a", decodeCopyRequest(t, `{"target_scope":"workspace","target_workspace_id":"workspace-b","name":"to-b"}`))
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if item.WorkspaceID != "workspace-b" {
		t.Fatalf("item workspace = %q", item.WorkspaceID)
	}
}

// TestServiceMove_WorkspaceToGlobalRemovesSource verifies moving a workspace secret to global removes the source.
func TestServiceMove_WorkspaceToGlobalRemovesSource(t *testing.T) {
	svc := testCopyMoveService(t, map[string]bool{"workspace-a": true})
	ctx := context.Background()
	w := mustCreateViaService(t, svc, "ws", "v4", ScopeWorkspace, "workspace-a")

	item, err := svc.Move(ctx, w.ID, "workspace-a", decodeCopyRequest(t, `{"target_scope":"global","name":"moved"}`))
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if item.Scope != ScopeGlobal {
		t.Fatalf("item scope = %q", item.Scope)
	}
	if _, err := svc.GetWorkspaceSecret(ctx, w.ID, "workspace-a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source still present after move: %v", err)
	}
}

// TestServiceMove_GlobalToWorkspaceRemovesSource verifies moving a global secret into a workspace removes the source.
func TestServiceMove_GlobalToWorkspaceRemovesSource(t *testing.T) {
	svc := testCopyMoveService(t, map[string]bool{"workspace-a": true})
	ctx := context.Background()
	g := mustCreateViaService(t, svc, "g", "v5", ScopeGlobal, "")

	item, err := svc.Move(ctx, g.ID, "", decodeCopyRequest(t, `{"target_scope":"workspace","target_workspace_id":"workspace-a"}`))
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if item.WorkspaceID != "workspace-a" {
		t.Fatalf("item workspace = %q", item.WorkspaceID)
	}
	if _, err := svc.Get(ctx, g.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("source still present after move: %v", err)
	}
}

// TestServiceCopy_NamePresenceSemantics verifies null, empty, whitespace, and over-long target names are rejected as ErrSecretValidation.
func TestServiceCopy_NamePresenceSemantics(t *testing.T) {
	svc := testCopyMoveService(t, map[string]bool{"workspace-a": true})
	ctx := context.Background()
	g := mustCreateViaService(t, svc, "API Key", "v", ScopeGlobal, "")

	t.Run("null is a validation error", func(t *testing.T) {
		_, err := svc.Copy(ctx, g.ID, "", decodeCopyRequest(t, `{"target_scope":"workspace","target_workspace_id":"workspace-a","name":null}`))
		if !errors.Is(err, ErrSecretValidation) {
			t.Fatalf("err = %v, want ErrSecretValidation", err)
		}
	})
	t.Run("empty string is a validation error", func(t *testing.T) {
		_, err := svc.Copy(ctx, g.ID, "", decodeCopyRequest(t, `{"target_scope":"workspace","target_workspace_id":"workspace-a","name":""}`))
		if !errors.Is(err, ErrSecretValidation) {
			t.Fatalf("err = %v, want ErrSecretValidation", err)
		}
	})
	t.Run("whitespace is a validation error", func(t *testing.T) {
		_, err := svc.Copy(ctx, g.ID, "", decodeCopyRequest(t, `{"target_scope":"workspace","target_workspace_id":"workspace-a","name":"   "}`))
		if !errors.Is(err, ErrSecretValidation) {
			t.Fatalf("err = %v, want ErrSecretValidation", err)
		}
	})
	t.Run("over 100 UTF-8 bytes is a validation error", func(t *testing.T) {
		long := ""
		for len(long) <= 100 {
			long += "é" // 2 bytes per code point
		}
		req := decodeCopyRequest(t, fmt.Sprintf(`{"target_scope":"workspace","target_workspace_id":"workspace-a","name":%q}`, long))
		_, err := svc.Copy(ctx, g.ID, "", req)
		if !errors.Is(err, ErrSecretValidation) {
			t.Fatalf("err = %v, want ErrSecretValidation", err)
		}
	})
}

// TestServiceCopy_SameScopeRejected verifies copying within the same scope returns ErrSecretValidation.
func TestServiceCopy_SameScopeRejected(t *testing.T) {
	svc := testCopyMoveService(t, map[string]bool{"workspace-a": true})
	ctx := context.Background()
	g := mustCreateViaService(t, svc, "g", "v", ScopeGlobal, "")
	w := mustCreateViaService(t, svc, "w", "v", ScopeWorkspace, "workspace-a")

	if _, err := svc.Copy(ctx, g.ID, "", decodeCopyRequest(t, `{"target_scope":"global"}`)); !errors.Is(err, ErrSecretValidation) {
		t.Fatalf("global->global err = %v, want ErrSecretValidation", err)
	}
	if _, err := svc.Copy(ctx, w.ID, "workspace-a", decodeCopyRequest(t, `{"target_scope":"workspace","target_workspace_id":"workspace-a"}`)); !errors.Is(err, ErrSecretValidation) {
		t.Fatalf("workspace A->A err = %v, want ErrSecretValidation", err)
	}
}

// TestServiceCopy_NameConflict verifies a target-name collision in the destination scope returns ErrSecretNameConflict.
func TestServiceCopy_NameConflict(t *testing.T) {
	svc := testCopyMoveService(t, map[string]bool{"workspace-a": true})
	ctx := context.Background()
	// "taken" lives in the workspace target scope.
	mustCreateViaService(t, svc, "taken", "other", ScopeWorkspace, "workspace-a")
	g := mustCreateViaService(t, svc, "g", "v", ScopeGlobal, "")

	_, err := svc.Copy(ctx, g.ID, "", decodeCopyRequest(t, `{"target_scope":"workspace","target_workspace_id":"workspace-a","name":"taken"}`))
	if !errors.Is(err, ErrSecretNameConflict) {
		t.Fatalf("err = %v, want ErrSecretNameConflict", err)
	}
}

// TestServiceCopy_MissingSourceIsNotFound verifies an unknown source ID returns ErrNotFound.
func TestServiceCopy_MissingSourceIsNotFound(t *testing.T) {
	svc := testCopyMoveService(t, nil)
	_, err := svc.Copy(context.Background(), "missing", "", decodeCopyRequest(t, `{"target_scope":"global"}`))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestServiceCopy_UnauthorizedSourceIsDenied verifies revoking source-workspace access makes Copy fail with ErrWorkspaceAccessDenied.
func TestServiceCopy_UnauthorizedSourceIsDenied(t *testing.T) {
	svc := testCopyMoveService(t, map[string]bool{"workspace-a": true})
	ctx := context.Background()
	w := mustCreateViaService(t, svc, "w", "v", ScopeWorkspace, "workspace-a")

	// Revoke access to the source workspace after seeding the secret.
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return ErrWorkspaceAccessDenied })
	svc.SetWorkspaceExistenceChecker(func(context.Context, string) error { return ErrWorkspaceAccessDenied })

	_, err := svc.Copy(ctx, w.ID, "workspace-a", decodeCopyRequest(t, `{"target_scope":"global"}`))
	if !errors.Is(err, ErrWorkspaceAccessDenied) {
		t.Fatalf("err = %v, want ErrWorkspaceAccessDenied", err)
	}
}

// TestServiceCopy_UnauthorizedDestinationIsDenied verifies an unallowed destination workspace returns ErrWorkspaceAccessDenied.
func TestServiceCopy_UnauthorizedDestinationIsDenied(t *testing.T) {
	svc := testCopyMoveService(t, map[string]bool{"workspace-a": true}) // workspace-b not allowed
	ctx := context.Background()
	g := mustCreateViaService(t, svc, "g", "v", ScopeGlobal, "")

	_, err := svc.Copy(ctx, g.ID, "", decodeCopyRequest(t, `{"target_scope":"workspace","target_workspace_id":"workspace-b"}`))
	if !errors.Is(err, ErrWorkspaceAccessDenied) {
		t.Fatalf("err = %v, want ErrWorkspaceAccessDenied", err)
	}
}

// TestServiceCopy_NilExistenceCheckerFailsClosed verifies a nil existence checker denies workspace targets (fails closed).
func TestServiceCopy_NilExistenceCheckerFailsClosed(t *testing.T) {
	store := newTestSQLiteStore(t)
	svc := NewService(NewUserVisibleStore(store), nil)
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	ctx := context.Background()
	g := mustCreateViaService(t, svc, "g", "v", ScopeGlobal, "")

	_, err := svc.Copy(ctx, g.ID, "", decodeCopyRequest(t, `{"target_scope":"workspace","target_workspace_id":"any"}`))
	if !errors.Is(err, ErrWorkspaceAccessDenied) {
		t.Fatalf("err = %v, want ErrWorkspaceAccessDenied (nil checker fails closed)", err)
	}
}

// TestServiceCopy_RawAuthorizerErrorPassesThroughUnclassified verifies a raw authorizer error is returned as-is rather than mapped to a sentinel.
func TestServiceCopy_RawAuthorizerErrorPassesThroughUnclassified(t *testing.T) {
	store := newTestSQLiteStore(t)
	svc := NewService(NewUserVisibleStore(store), nil)
	rawErr := errors.New("database unreachable")
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return rawErr })
	svc.SetWorkspaceExistenceChecker(func(context.Context, string) error { return nil })
	ctx := context.Background()
	g := mustCreateViaService(t, svc, "g", "v", ScopeGlobal, "")

	_, err := svc.Copy(ctx, g.ID, "", decodeCopyRequest(t, `{"target_scope":"workspace","target_workspace_id":"workspace-a"}`))
	if !errors.Is(err, rawErr) {
		t.Fatalf("err = %v, want the raw authorizer error (unclassified)", err)
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrSecretValidation) || errors.Is(err, ErrWorkspaceAccessDenied) {
		t.Fatalf("raw error misclassified as a sentinel: %v", err)
	}
}

// TestServiceCopy_InternalSourceIDIsNotFound verifies internal (backend-owned) source IDs are rejected as ErrNotFound.
func TestServiceCopy_InternalSourceIDIsNotFound(t *testing.T) {
	svc := testCopyMoveService(t, map[string]bool{"workspace-a": true})
	_, err := svc.Copy(context.Background(), "github:user:workspace:user:access", "", decodeCopyRequest(t, `{"target_scope":"workspace","target_workspace_id":"workspace-a"}`))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestServiceMove_CrossUserOwnershipEnforced verifies a user cannot move another user's workspace secret (ErrNotFound) while the owner can.
func TestServiceMove_CrossUserOwnershipEnforced(t *testing.T) {
	svc := testCopyMoveService(t, map[string]bool{"workspace-a": true})
	alice := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-a"})
	bob := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-b"})

	// Alice creates a workspace secret.
	created, err := svc.Create(alice, &CreateSecretRequest{Name: "alice-ws", Value: "v", Scope: ScopeWorkspace, WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatalf("create as alice: %v", err)
	}
	// Bob cannot move it (store-level per-user scoping).
	if _, err := svc.Move(bob, created.ID, "workspace-a", decodeCopyRequest(t, `{"target_scope":"global"}`)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob move err = %v, want ErrNotFound", err)
	}
	// Alice can.
	if _, err := svc.Move(alice, created.ID, "workspace-a", decodeCopyRequest(t, `{"target_scope":"global","name":"alice-global"}`)); err != nil {
		t.Fatalf("alice move: %v", err)
	}
}

// mustCreateViaService is a small helper that returns the created item.
func mustCreateViaService(t *testing.T, svc *Service, name, value string, scope SecretScope, workspaceID string) *SecretListItem {
	t.Helper()
	item, err := svc.Create(context.Background(), &CreateSecretRequest{Name: name, Value: value, Scope: scope, WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return item
}

// globalID resolves a global secret's ID by name via List.
func globalID(t *testing.T, svc *Service, name string) string {
	t.Helper()
	items, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, item := range items {
		if item.Name == name {
			return item.ID
		}
	}
	t.Fatalf("no global secret named %s", name)
	return ""
}
