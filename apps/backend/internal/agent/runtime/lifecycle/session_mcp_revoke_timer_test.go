package lifecycle

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

// blockingRevoker holds every revoke attempt until its context ends.
type blockingRevoker struct {
	fakeControl
}

func (b *blockingRevoker) RevokeMCPToken(ctx context.Context, id string) error {
	b.revokes++
	<-ctx.Done()
	return ctx.Err()
}

// The wait between revoke attempts ends with the revoke deadline: once the
// budget is spent no further attempt starts, however long the gap is.
func TestRevokeMCPTokenStopsWaitingWhenTheBudgetEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctl := &blockingRevoker{}
		const attempts, gap = 3, time.Hour
		start := time.Now()
		if revokeMCPToken(context.Background(), ctl, "exec-1", attempts, gap) {
			t.Fatal("a revoke that never answered reported success")
		}
		if ctl.revokes != 1 {
			t.Fatalf("revokes = %d, want 1: an attempt started after the budget ended", ctl.revokes)
		}
		if got, want := time.Since(start), attempts*(gap+revokeAttemptTimeout); got != want {
			t.Fatalf("returned after %v, want the %v revoke budget", got, want)
		}
	})
}
