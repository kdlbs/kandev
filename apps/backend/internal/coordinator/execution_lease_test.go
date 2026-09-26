package coordinator

import (
	"testing"
	"time"
)

func TestHostExecutionLeaseMatchesOnlyItsExactScope(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
	lease := HostExecutionLease{
		ID: "lease-1",
		HostExecutionLeaseScope: HostExecutionLeaseScope{
			GrantID:      "grant-1",
			PrincipalID:  "principal-1",
			WorkspaceID:  "workspace-1",
			TargetTaskID: "task-1",
			RepositoryID: "repository-1",
			Branch:       "feature/example",
			ExpectedHead: "0123456789abcdef0123456789abcdef01234567",
			Operation:    HostOperationGitPushFastForward,
		},
		ExpiresAt: now.Add(time.Minute),
	}

	if err := lease.ValidateFor(lease.HostExecutionLeaseScope, now); err != nil {
		t.Fatalf("ValidateFor(exact scope) = %v, want nil", err)
	}

	for name, mutate := range map[string]func(*HostExecutionLeaseScope){
		"workspace":  func(scope *HostExecutionLeaseScope) { scope.WorkspaceID = "workspace-2" },
		"repository": func(scope *HostExecutionLeaseScope) { scope.RepositoryID = "repository-2" },
		"branch":     func(scope *HostExecutionLeaseScope) { scope.Branch = "main" },
		"head": func(scope *HostExecutionLeaseScope) {
			scope.ExpectedHead = "abcdef0123456789abcdef0123456789abcdef0123"
		},
		"operation": func(scope *HostExecutionLeaseScope) { scope.Operation = HostOperationGitMergeFastForward },
	} {
		t.Run(name, func(t *testing.T) {
			scope := lease.HostExecutionLeaseScope
			mutate(&scope)
			if err := lease.ValidateFor(scope, now); err == nil {
				t.Fatal("ValidateFor(mismatched scope) = nil, want denial")
			}
		})
	}
}

func TestHostExecutionLeaseDeniesExpiryAndRevocation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
	scope := HostExecutionLeaseScope{
		GrantID: "grant-1", PrincipalID: "principal-1", WorkspaceID: "workspace-1",
		TargetTaskID: "task-1", RepositoryID: "repository-1", Branch: "feature/example",
		ExpectedHead: "0123456789abcdef0123456789abcdef01234567", Operation: HostOperationGitIndexUpdate,
	}

	for name, lease := range map[string]HostExecutionLease{
		"expired": {ID: "lease-expired", HostExecutionLeaseScope: scope, ExpiresAt: now},
		"revoked": {ID: "lease-revoked", HostExecutionLeaseScope: scope, ExpiresAt: now.Add(time.Minute), RevokedAt: &now},
	} {
		t.Run(name, func(t *testing.T) {
			if err := lease.ValidateFor(scope, now); err == nil {
				t.Fatal("ValidateFor(inactive lease) = nil, want denial")
			}
		})
	}
}
