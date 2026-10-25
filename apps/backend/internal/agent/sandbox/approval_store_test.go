package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func approvalStoreFixture(t *testing.T, database *sqlx.DB) (*Service, *Attempt, string) {
	t.Helper()
	policy := policyFixture(t)
	return approvalPolicyFixture(t, database, policy, policy)
}

func approvalPolicyFixture(t *testing.T, database *sqlx.DB, policy, ceiling Policy) (*Service, *Attempt, string) {
	t.Helper()
	ctx := operatorContext(t)
	service, err := NewService(database, true, nil)
	require.NoError(t, err)
	service.SetTargetResolver(func(context.Context, string, string) (TargetScope, error) { return TargetScope{OrgID: "org"}, nil })
	service.SetSessionAuthorizer(func(ctx context.Context, sessionID string) error {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok || identity.OrgID != "org" || sessionID != "session" {
			return ErrNotFound
		}
		return nil
	})
	file := filepath.Join(policy.Grants.Filesystem[0].Path, "file")
	require.NoError(t, os.WriteFile(file, []byte("fixture"), 0600))
	revision, err := service.Publish(ctx, policy, 0)
	require.NoError(t, err)
	ceilingRevision := revision
	if ceiling.ID != policy.ID {
		ceilingRevision, err = service.Publish(ctx, ceiling, 0)
		require.NoError(t, err)
	}
	_, err = service.PutBinding(ctx, Binding{Scope: ScopeExecutor, TargetID: "local", Executor: &ExecutorPolicy{Mode: ModeRequired, InstallationID: policy.InstallationID, ProviderKey: policy.ProviderKey, CeilingRevisionID: ceilingRevision.ID, DefaultRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}}}, 0)
	require.NoError(t, err)
	selection := SelectionContext{OrgID: "org", ExecutorID: "local"}
	resolved, err := service.Resolve(ctx, selection)
	require.NoError(t, err)
	content := []byte("fixture")
	digest := sha256.Sum256(content)
	runtimeDigest := hex.EncodeToString(digest[:])
	files, err := contract.MaterializeRuntime(ctx, t.TempDir(), content, runtimeDigest, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, files.Close()) })
	launch := &contract.Launch{Version: 1, AttemptID: "approval-attempt", Generation: 1, PolicyDigest: revision.Digest, PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests}
	attempt, err := service.CaptureAttempt(ctx, CaptureRequest{Selection: selection, Resolved: resolved, SessionID: "session", ExecutionID: "execution", WorkDir: policy.Grants.Filesystem[0].Path, Launch: launch, Provider: AttemptProvider{PluginID: "plugin", InstallationID: policy.InstallationID, Key: policy.ProviderKey, Version: "1", RuntimeVersion: "1", PackageDigest: launch.PackageDigest, RuntimeDigest: runtimeDigest}})
	require.NoError(t, err)
	attempt, err = service.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, AttemptStarting, nil)
	require.NoError(t, err)
	attempt, err = service.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, AttemptRunning, &AttemptOwner{SupervisorPID: 100, SupervisorBirth: "supervisor-birth", ChildPID: 101, ChildBirth: "child-birth", ProviderSessionID: "provider-session", AgentctlIdentity: "runtime"})
	require.NoError(t, err)
	return service, attempt, file
}

func TestSandboxApprovalStoreSQLite(t *testing.T) { testApprovalStore(t, sqlitePolicyDB(t)) }
func TestSandboxApprovalStorePostgres(t *testing.T) {
	testApprovalStore(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func testApprovalStore(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, attempt, file := approvalStoreFixture(t, database)
	ctx := operatorContext(t)
	operation := ApprovalOperation{RequestID: "request", ProviderSessionID: "provider-session", ChildPID: 101, Access: "read", Target: file}
	approval, err := service.RecordApproval(ctx, "org", attempt.ID, operation, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.NoError(t, err)
	duplicate, err := service.RecordApproval(ctx, "org", attempt.ID, operation, strings.Repeat("a", 64), time.Now().Add(40*time.Second))
	require.NoError(t, err)
	require.Equal(t, approval, duplicate)
	_, err = service.RecordApproval(ctx, "org", attempt.ID, operation, strings.Repeat("b", 64), time.Now().Add(30*time.Second))
	require.ErrorIs(t, err, ErrConflict)
	_, err = service.DecideApproval(ctx, attempt.ID, operation.RequestID, approval.Version, "allow_once")
	require.Error(t, err, "no live callback guard must refuse grants")
	service.SetApprovalGuard(func(context.Context, *Attempt, *Approval) error { return nil })
	for _, caller := range []context.Context{
		authn.WithIdentity(t.Context(), authn.Identity{UserID: "agent", OrgID: "org", Role: authn.RoleAdmin, TokenID: "pat"}),
		scope.WithPrincipal(ctx, scope.Principal{CallerTaskID: "task", CallerSessionID: "session"}),
	} {
		_, err = service.DecideApproval(caller, attempt.ID, operation.RequestID, approval.Version, "allow_once")
		require.ErrorIs(t, err, ErrAuthority)
	}
	decided, err := service.DecideApproval(ctx, attempt.ID, operation.RequestID, approval.Version, "allow_once")
	require.NoError(t, err)
	require.Equal(t, ApprovalApproved, decided.State)
	require.Equal(t, "operator", decided.OperatorID)
	_, err = service.DecideApproval(ctx, attempt.ID, operation.RequestID, approval.Version, "deny")
	require.ErrorIs(t, err, ErrConflict)
	claimed, err := service.ClaimApproval(ctx, "org", attempt.ID, operation.RequestID, decided.Version)
	require.NoError(t, err)
	require.Equal(t, ApprovalResponding, claimed.State)
	_, err = service.ClaimApproval(ctx, "org", attempt.ID, operation.RequestID, claimed.Version)
	require.Error(t, err, "approval cannot be replayed")
	operation.RequestID = "outside"
	operation.Target = filepath.Join(t.TempDir(), "foreign")
	operation.Access = "write"
	_, err = service.RecordApproval(ctx, "org", attempt.ID, operation, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.Error(t, err)
	operation.RequestID = "foreign-session"
	operation.Target = file
	operation.ProviderSessionID = "another"
	_, err = service.RecordApproval(ctx, "org", attempt.ID, operation, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.Error(t, err)
	retained, err := service.ReadAttempt(ctx, "org", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, attempt.Snapshot, retained.Snapshot, "approval must not change the policy")
}
