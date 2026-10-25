package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestSandboxApprovalChangesSQLite(t *testing.T) { testApprovalChanges(t, sqlitePolicyDB(t)) }
func TestSandboxApprovalChangesPostgres(t *testing.T) {
	testApprovalChanges(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func testApprovalChanges(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, attempt, file := approvalStoreFixture(t, database)
	service.SetApprovalGuard(func(context.Context, *Attempt, *Approval) error { return nil })
	ctx := operatorContext(t)
	operation := ApprovalOperation{RequestID: "request", ProviderSessionID: "provider-session", ChildPID: 101, Access: "read", Target: file}
	states := []string{}
	service.SetApprovalChangeNotifier(func(ctx context.Context, orgID, attemptID string) {
		require.Equal(t, "org", orgID)
		require.Equal(t, attempt.ID, attemptID)
		current, err := service.ReadApproval(ctx, orgID, attemptID, operation.RequestID)
		require.NoError(t, err, "notification must follow the committed write")
		states = append(states, current.State)
	})
	record := func() *Approval {
		t.Helper()
		value, err := service.RecordApproval(ctx, "org", attempt.ID, operation, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
		require.NoError(t, err)
		return value
	}
	first := record()
	require.Equal(t, []string{ApprovalPending}, states)
	record()
	require.Len(t, states, 1, "idempotent replay does not announce a change")
	approved, err := service.DecideApproval(ctx, attempt.ID, operation.RequestID, first.Version, "allow_once")
	require.NoError(t, err)
	_, err = service.DecideApproval(ctx, attempt.ID, operation.RequestID, first.Version, "deny")
	require.Error(t, err)
	require.Equal(t, []string{ApprovalPending, ApprovalApproved}, states)
	claimed, err := service.ClaimApproval(ctx, "org", attempt.ID, operation.RequestID, approved.Version)
	require.NoError(t, err)
	_, err = service.FinishApproval(ctx, "org", attempt.ID, operation.RequestID, claimed.Version, true)
	require.NoError(t, err)
	require.Equal(t, []string{ApprovalPending, ApprovalApproved, ApprovalResponding, ApprovalSent}, states)
	operation.RequestID = "cancelled"
	record()
	require.NoError(t, service.CloseApprovals(ctx, "org", attempt.ID))
	require.Equal(t, ApprovalUndelivered, states[len(states)-1])
	before := len(states)
	require.NoError(t, service.CloseApprovals(ctx, "org", attempt.ID))
	require.Len(t, states, before)
	operation.RequestID = "abandoned"
	record()
	require.NoError(t, service.abandonApproval(ctx, "org", attempt.ID, operation.RequestID))
	require.Equal(t, ApprovalUndelivered, states[len(states)-1])
}
