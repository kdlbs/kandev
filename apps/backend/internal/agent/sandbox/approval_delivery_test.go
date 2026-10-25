package sandbox

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestSandboxApprovalDeliverySQLite(t *testing.T) { testApprovalDelivery(t, sqlitePolicyDB(t)) }
func TestSandboxApprovalDeliveryPostgres(t *testing.T) {
	testApprovalDelivery(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func testApprovalDelivery(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, attempt, file := approvalStoreFixture(t, database)
	ctx := operatorContext(t)
	live := true
	service.SetApprovalGuard(func(context.Context, *Attempt, *Approval) error {
		if !live {
			return ErrConflict
		}
		return nil
	})
	record := func(id string) *Approval {
		t.Helper()
		approval, err := service.RecordApproval(ctx, "org", attempt.ID, ApprovalOperation{RequestID: id, ProviderSessionID: "provider-session", ChildPID: 101, Access: "read", Target: file}, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
		require.NoError(t, err)
		return approval
	}
	approve := func(a *Approval) *Approval {
		t.Helper()
		next, err := service.DecideApproval(ctx, attempt.ID, a.Snapshot.Operation.RequestID, a.Version, "allow_once")
		require.NoError(t, err)
		return next
	}
	a := approve(record("lost-callback"))
	live = false
	_, err := service.ClaimApproval(ctx, "org", attempt.ID, a.Snapshot.Operation.RequestID, a.Version)
	require.Error(t, err, "callback loss after an operator decision must refuse delivery")
	live = true
	a = approve(record("sent"))
	a, err = service.ClaimApproval(ctx, "org", attempt.ID, "sent", a.Version)
	require.NoError(t, err)
	a, err = service.FinishApproval(ctx, "org", attempt.ID, "sent", a.Version, true)
	require.NoError(t, err)
	require.Equal(t, ApprovalSent, a.State)
	_, err = service.FinishApproval(ctx, "org", attempt.ID, "sent", a.Version, false)
	require.ErrorIs(t, err, ErrConflict)
	a = approve(record("uncertain"))
	a, err = service.ClaimApproval(ctx, "org", attempt.ID, "uncertain", a.Version)
	require.NoError(t, err)
	a, err = service.FinishApproval(ctx, "org", attempt.ID, "uncertain", a.Version, false)
	require.NoError(t, err)
	require.Equal(t, ApprovalUndelivered, a.State)
	_, err = service.ClaimApproval(ctx, "org", attempt.ID, "uncertain", a.Version)
	require.ErrorIs(t, err, ErrConflict)
	pending := record("stopped")
	approved := approve(record("approved-before-stop"))
	_, err = service.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, AttemptStopped, nil)
	require.NoError(t, err)
	_, err = service.DecideApproval(ctx, attempt.ID, "stopped", pending.Version, "allow_once")
	require.Error(t, err)
	_, err = service.ClaimApproval(ctx, "org", attempt.ID, "approved-before-stop", approved.Version)
	require.Error(t, err)
	require.NoError(t, service.CloseApprovals(ctx, "org", attempt.ID))
	closed, err := service.ReadApproval(ctx, "org", attempt.ID, "stopped")
	require.NoError(t, err)
	require.Equal(t, ApprovalUndelivered, closed.State)
	sent, err := service.ReadApproval(ctx, "org", attempt.ID, "sent")
	require.NoError(t, err)
	require.Equal(t, ApprovalSent, sent.State, "closing an attempt must preserve its delivered history")
}

type failedApprovalFlush struct{ *httptest.ResponseRecorder }

func (*failedApprovalFlush) FlushError() error { return io.ErrClosedPipe }

func TestSandboxApprovalFlushFailureCannotClaimDelivery(t *testing.T) {
	service, attempt, file := approvalStoreFixture(t, sqlitePolicyDB(t))
	service.SetApprovalGuard(func(context.Context, *Attempt, *Approval) error { return nil })
	operation := ApprovalOperation{RequestID: "flush-failed", ProviderSessionID: "provider-session", ChildPID: 101, Access: "read", Target: file}
	approval, err := service.RecordApproval(t.Context(), "org", attempt.ID, operation, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.NoError(t, err)
	approval, err = service.DecideApproval(operatorContext(t), attempt.ID, operation.RequestID, approval.Version, "allow_once")
	require.NoError(t, err)
	channel := &ApprovalChannel{owner: &ApprovalChannels{service: service}, key: approvalChannelKey{"org", attempt.ID}, ctx: t.Context(), replies: ApprovalReplies{Allow: `{"status":"granted"}`}}
	channel.deliver(t.Context(), &failedApprovalFlush{httptest.NewRecorder()}, approval)
	retained, err := service.ReadApproval(t.Context(), "org", attempt.ID, operation.RequestID)
	require.NoError(t, err)
	require.Equal(t, ApprovalUndelivered, retained.State)
	_, err = service.ClaimApproval(t.Context(), "org", attempt.ID, operation.RequestID, retained.Version)
	require.ErrorIs(t, err, ErrConflict)
}

func TestSandboxApprovalStopAtDecisionBoundaryCannotGrant(t *testing.T) {
	service, attempt, file := approvalStoreFixture(t, sqlitePolicyDB(t))
	operation := ApprovalOperation{RequestID: "stop-race", ProviderSessionID: "provider-session", ChildPID: 101, Access: "read", Target: file}
	approval, err := service.RecordApproval(t.Context(), "org", attempt.ID, operation, strings.Repeat("a", 64), time.Now().Add(30*time.Second))
	require.NoError(t, err)
	service.SetApprovalGuard(func(ctx context.Context, current *Attempt, _ *Approval) error {
		_, stopErr := service.TransitionAttempt(ctx, "org", attempt.ID, current.Version, AttemptStopped, nil)
		return stopErr
	})
	_, err = service.DecideApproval(operatorContext(t), attempt.ID, operation.RequestID, approval.Version, "allow_once")
	require.ErrorIs(t, err, ErrConflict)
}
