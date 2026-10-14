package coordinator

import (
	"context"
	"testing"
	"time"
)

func TestDuplicateApprovalSweepReleasesUnusedTicker(t *testing.T) {
	_, _, _, svc := approveFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	activeStopped := make(chan struct{})
	discardedStopped := make(chan struct{})
	svc.startApprovalSweep(ctx, make(chan time.Time), func() { close(activeStopped) })
	svc.startApprovalSweep(ctx, make(chan time.Time), func() { close(discardedStopped) })
	select {
	case <-discardedStopped:
	default:
		t.Error("duplicate start retained its unused ticker")
	}
	cancel()
	svc.WaitApprovalSweepStopped()
	select {
	case <-activeStopped:
	default:
		t.Error("active sweep was not joined")
	}
}

func TestDuplicateRetentionReleasesUnusedTicker(t *testing.T) {
	_, _, _, svc := approveFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	activeStopped := make(chan struct{})
	discardedStopped := make(chan struct{})
	svc.startActivityRetention(ctx, make(chan time.Time), func() { close(activeStopped) })
	svc.startActivityRetention(ctx, make(chan time.Time), func() { close(discardedStopped) })
	select {
	case <-discardedStopped:
	default:
		t.Error("duplicate retention retained its ticker and launched another worker")
	}
	cancel()
	svc.WaitActivityRetentionStopped()
	select {
	case <-activeStopped:
	default:
		t.Error("active retention was not joined")
	}
}
