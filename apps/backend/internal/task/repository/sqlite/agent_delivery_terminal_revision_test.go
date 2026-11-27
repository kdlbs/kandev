package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestTerminalSettlementAdvancesRecoveryBlockProjection(t *testing.T) {
	testTerminalSettlementRevision(t, newRepoForSessionTests(t))
}

func TestPostgresTerminalSettlementAdvancesRecoveryBlockProjection(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	testTerminalSettlementRevision(t, repo)
}

func testTerminalSettlementRevision(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	seedDeliverySettlement(t, repo, ctx, "revision", "submission-revision")
	before, err := repo.GetTaskSession(ctx, "session-delivery-revision")
	if err != nil {
		t.Fatal(err)
	}
	clock := before.UpdatedAt.Add(time.Second)
	repo.clockNow = func() time.Time { return clock }
	event := terminalDeliveryEvent("revision", "submission-revision", "stream-revision", 1, "complete")
	projectDeliveryEvent(t, repo, ctx, event)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCompleted, clock)
	if err != nil || !settled {
		t.Fatalf("settlement = %v, %v", settled, err)
	}
	after, err := repo.GetTaskSession(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.UpdatedAt.Equal(clock) {
		t.Fatalf("recovery fence = %s, want %s", after.UpdatedAt, clock)
	}
}
