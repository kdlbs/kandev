package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestTypedPolicyAttemptSQLite(t *testing.T) {
	testTypedPolicyAttempt(t, sqlitePolicyDB(t))
}

func TestTypedPolicyAttemptPostgres(t *testing.T) {
	testTypedPolicyAttempt(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func testTypedPolicyAttempt(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, request, file := typedPolicyCaptureFixture(t, database)
	ctx := operatorContext(t)
	legacy := request
	legacy.Provider.ProtocolMajor = 1
	_, err := service.CaptureAttempt(ctx, legacy)
	require.ErrorIs(t, err, ErrIneligible, "a schema 2 capture must not use a legacy provider")
	attempt, err := service.CaptureAttempt(ctx, request)
	require.NoError(t, err)
	require.Equal(t, 2, attempt.Snapshot.SchemaVersion)
	require.NotNil(t, attempt.Snapshot.PolicyV2)
	require.Equal(t, []contract.FilesystemGrant{{Path: request.WorkDir}}, attempt.Snapshot.Grants.Filesystem)
	require.Equal(t, []contract.FilesystemGrant{{Path: file, Writable: true}}, attempt.Snapshot.PolicyV2.ApprovalEnvelope.Filesystem)
	require.Empty(t, attempt.Snapshot.PolicyV2.ApprovalEnvelope.TCP)
	reloaded, err := NewService(database, true, nil)
	require.NoError(t, err)
	attempt, err = reloaded.ReadAttempt(ctx, "org", attempt.ID)
	require.NoError(t, err)
	attempt, err = reloaded.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, AttemptStarting, nil)
	require.NoError(t, err)
	attempt, err = reloaded.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, AttemptRunning,
		&AttemptOwner{SupervisorPID: 100, SupervisorBirth: "supervisor-birth", ChildPID: 101,
			ChildBirth: "child-birth", ProviderSessionID: "provider-session", AgentctlIdentity: "runtime"})
	require.NoError(t, err)
	operation := ApprovalOperation{RequestID: "typed-write", ProviderSessionID: "provider-session", ChildPID: 101, Access: filesystemWrite, Target: file}
	_, err = reloaded.RecordApproval(ctx, "org", attempt.ID, operation, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.NoError(t, err)
	operation.RequestID = "outside-envelope"
	operation.Target = filepath.Join(request.WorkDir, "sibling")
	require.NoError(t, os.WriteFile(operation.Target, []byte("sibling"), 0600))
	_, err = reloaded.RecordApproval(ctx, "org", attempt.ID, operation, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.ErrorIs(t, err, contract.ErrOutsideCeiling, "executor headroom does not create schema 2 approval eligibility")
}

func typedPolicyCaptureFixture(t *testing.T, database *sqlx.DB) (*Service, CaptureRequest, string) {
	t.Helper()
	service, original, file := approvalStoreFixture(t, database, 2)
	ctx := operatorContext(t)
	_, err := service.TransitionAttempt(ctx, "org", original.ID, original.Version, AttemptStopped, nil)
	require.NoError(t, err)
	policy := typedPolicyFixture()
	policy.ID = "typed-capture"
	policy.PolicyV2.StaticGrants.TCP = nil
	policy.PolicyV2.StaticGrants.Filesystem = []contract.FilesystemGrant{{Path: original.Snapshot.WorkDir}}
	policy.PolicyV2.ApprovalEnvelope.Filesystem = []contract.FilesystemGrant{{Path: file, Writable: true}}
	revision, err := service.Publish(ctx, policy, 0)
	require.NoError(t, err)
	maximum := typedPolicyFixture()
	maximum.ID = "typed-capture-ceiling"
	maximum.PolicyV2.StaticGrants.Filesystem = []contract.FilesystemGrant{{Path: original.Snapshot.WorkDir, Writable: true}}
	maximum.PolicyV2.StaticGrants.KandevMCP = true
	ceiling, err := service.Publish(ctx, maximum, 0)
	require.NoError(t, err)
	binding, err := service.GetBinding(ctx, ScopeExecutor, "local")
	require.NoError(t, err)
	binding.Executor.DefaultRevisionID, binding.Executor.CeilingRevisionID = revision.ID, ceiling.ID
	binding.Executor.EligibleRevisionIDs = []string{revision.ID}
	_, err = service.PutBinding(ctx, *binding, binding.Version)
	require.NoError(t, err)
	selection := SelectionContext{OrgID: "org", ExecutorID: "local"}
	resolved, err := service.Resolve(ctx, selection)
	require.NoError(t, err)
	launch := original.Snapshot.Launch.Clone()
	launch.AttemptID, launch.PolicyDigest = "typed-attempt", revision.Digest
	return service, CaptureRequest{Selection: selection, Resolved: resolved, SessionID: "session", ExecutionID: "typed-execution",
		WorkDir: original.Snapshot.WorkDir, Launch: launch, Provider: original.Snapshot.Provider}, file
}
