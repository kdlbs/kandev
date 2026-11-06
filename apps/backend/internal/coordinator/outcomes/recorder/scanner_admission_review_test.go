package recorder

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestScannerDailyAdmissionPreservesOrdinaryStoppedContract(t *testing.T) {
	s := NewScanner(&Capture{now: time.Now}, nil, nil)
	s.Stop()
	if s.enqueueWaiting(context.Background(), "daily-task") {
		t.Fatal("daily admission succeeded after Stop")
	}
	if !s.EnqueueScan("ordinary-task") {
		t.Fatal("ordinary stopped admission was counted as a full-queue drop")
	}
	if len(s.order) != 0 || len(s.queued) != 0 {
		t.Fatal("stopped admission changed the queue")
	}
}

func TestScannerDailyWaitingEndsWithoutAdmission(t *testing.T) {
	for _, stop := range []bool{false, true} {
		name := "cancel"
		if stop {
			name = "stop"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := NewScanner(&Capture{now: time.Now}, nil, nil)
				t.Cleanup(s.Stop)
				fillScanQueue(s)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan bool, 1)
				go func() { done <- s.enqueueWaiting(ctx, "daily-task") }()
				synctest.Wait()
				select {
				case <-done:
					t.Fatal("daily admission returned while the queue was full")
				default:
				}
				if stop {
					s.Stop()
				} else {
					cancel()
				}
				if <-done {
					t.Fatal("daily admission succeeded after its owner ended")
				}
				if len(s.order) != scanQueueCap || len(s.queued) != 0 {
					t.Fatal("cancelled admission changed the full queue")
				}
			})
		})
	}
}

func TestScannerDailyAdmissionDeduplicatesLiveWork(t *testing.T) {
	s := NewScanner(&Capture{now: time.Now}, nil, nil)
	t.Cleanup(s.Stop)
	for range 2 {
		if !s.enqueueWaiting(context.Background(), "daily-task") {
			t.Fatal("live daily admission failed")
		}
	}
	if len(s.order) != 1 || !s.queued["daily-task"] {
		t.Fatal("daily work was not queued exactly once")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s.enqueueWaiting(ctx, "daily-task") {
		t.Fatal("cancelled owner inherited another admission's success")
	}
	if len(s.order) != 1 || !s.queued["daily-task"] {
		t.Fatal("cancelled owner changed existing daily work")
	}
}
