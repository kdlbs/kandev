package recorder

import (
	"context"
	"testing"
	"time"
)

func (f *fixture) scanner(chk *fakeChecker, delays ...time.Duration) *Scanner {
	s := NewScanner(f.capture(chk), f.db, nil)
	if len(delays) > 0 {
		s.delays = delays
	}
	return s
}

func (s *Scanner) activeChains() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func TestScanner_LateHistoryRowFoundByRetryAndChainEnds(t *testing.T) {
	f := moveBackFixture(t)
	s := f.scanner(&fakeChecker{verdicts: map[string]Verdict{"mgr": VerdictManager}}, 20*time.Millisecond, 20*time.Millisecond)
	s.Start(context.Background())
	defer s.Stop()
	s.OnTaskMoved(context.Background(), "t1", f.at(-time.Minute))
	waitFor(t, "chain pending", func() bool { return s.activeChains() == 1 })
	f.history("s0", "mgr", f.at(-30*time.Second))
	waitFor(t, "late row judged", func() bool { return f.feedbackCount("moved_back") == 1 })
	waitFor(t, "chain ended", func() bool { return s.activeChains() == 0 })
}

func TestScanner_ChainEndsAfterLastDelayWithoutRow(t *testing.T) {
	f := moveBackFixture(t)
	s := f.scanner(&fakeChecker{}, 10*time.Millisecond)
	s.Start(context.Background())
	defer s.Stop()
	s.OnTaskMoved(context.Background(), "t1", f.at(-time.Minute))
	waitFor(t, "chain ended", func() bool { return s.activeChains() == 0 })
}

func TestScanner_TaskWithoutCoordinatorActionStartsNothing(t *testing.T) {
	f := newFixture(t)
	s := f.scanner(&fakeChecker{})
	s.OnTaskMoved(context.Background(), "unknown", f.at(0))
	if s.activeChains() != 0 || len(s.order) != 0 {
		t.Fatal("chain or scan for a task without coordinator action")
	}
}

func TestScanner_ChainWindowIsolatesTwoMoves(t *testing.T) {
	f := moveBackFixture(t)
	s := f.scanner(&fakeChecker{}, time.Hour)
	first, second := f.at(-10*time.Minute), f.at(-5*time.Minute)
	s.OnTaskMoved(context.Background(), "t1", first)
	s.OnTaskMoved(context.Background(), "t1", second)
	// A row at -3m lies in the second window only: the first chain keeps waiting.
	s.evaluate("t1", []time.Time{f.at(-3 * time.Minute)})
	chains := s.chains["t1"]
	if chains[0].ended || !chains[1].ended {
		t.Fatalf("ended = %v, %v", chains[0].ended, chains[1].ended)
	}
	s.Stop()
}

func TestScanner_ChainCapAndQueueFullAreCounted(t *testing.T) {
	f := moveBackFixture(t)
	s := f.scanner(&fakeChecker{})
	s.active = chainCap
	before := ScanDroppedCount(ScanDroppedChainCap)
	s.OnTaskMoved(context.Background(), "t1", f.at(0))
	if ScanDroppedCount(ScanDroppedChainCap) != before+1 {
		t.Fatal("chain cap not counted")
	}
	beforeQ := ScanDroppedCount(ScanDroppedQueueFull)
	for i := 0; i < scanQueueCap+1; i++ {
		s.EnqueueScan("t" + time.Duration(i).String())
	}
	if ScanDroppedCount(ScanDroppedQueueFull) <= beforeQ {
		t.Fatal("queue full not counted")
	}
}

func TestScanner_DailyScanFindsFinalAndRecentTasks(t *testing.T) {
	f := moveBackFixture(t)
	f.history("s0", "mgr", f.at(-time.Minute))
	f.exec(`INSERT INTO github_task_prs (id, task_id, state, merged_at) VALUES ('a', 't1', 'merged', ?)`, f.now)
	f.grade("p1", time.Time{}) // final row: the outcome sweep skips it
	s := f.scanner(&fakeChecker{verdicts: map[string]Verdict{"mgr": VerdictManager}})
	s.Start(context.Background())
	defer s.Stop()
	waitFor(t, "daily scan", func() bool { return f.feedbackCount("moved_back") == 1 })
}

func TestScanner_StopIsIdempotentAndCancelsTimers(t *testing.T) {
	f := moveBackFixture(t)
	s := f.scanner(&fakeChecker{}, time.Hour)
	s.Start(context.Background())
	s.OnTaskMoved(context.Background(), "t1", f.at(0))
	waitFor(t, "timer armed", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.chains["t1"]) == 1 && s.chains["t1"][0].timerPending
	})
	s.Stop()
	s.Stop()
}

func TestScanner_DroppedScanRearmsChainAndNoTimerAfterStop(t *testing.T) {
	f := moveBackFixture(t)
	s := f.scanner(&fakeChecker{}, time.Hour)
	for i := 0; i < scanQueueCap; i++ {
		s.order <- "x" + time.Duration(i).String()
	}
	s.OnTaskMoved(context.Background(), "t1", f.at(0))
	s.mu.Lock()
	armed := len(s.chains["t1"]) == 1 && s.chains["t1"][0].timerPending
	s.mu.Unlock()
	if !armed {
		t.Fatal("chain whose first scan was dropped has no retry timer")
	}
	s.Stop()
	late := &chain{task: "t2", at: f.at(0)}
	s.chains["t2"] = []*chain{late}
	s.evaluate("t2", nil)
	if late.timer != nil || late.timerPending {
		t.Fatal("timer armed after Stop")
	}
}

func fillScanQueue(s *Scanner) {
	for i := 0; i < scanQueueCap; i++ {
		s.order <- "filler-" + time.Duration(i).String()
	}
}

func TestScanner_DailyPassWaitsForRoomInsteadOfDropping(t *testing.T) {
	f := moveBackFixture(t)
	s := f.scanner(&fakeChecker{})
	fillScanQueue(s)
	dropped := ScanDroppedCount(ScanDroppedQueueFull)
	done := make(chan bool, 1)
	go func() { done <- s.enqueueWaiting(context.Background(), "late-task") }()
	select {
	case <-done:
		t.Fatal("enqueueWaiting returned while the queue was full")
	case <-time.After(3 * dailyEnqueueWait):
	}
	<-s.order
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("enqueueWaiting = false after room appeared")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("enqueueWaiting did not return once there was room")
	}
	s.mu.Lock()
	queued := s.queued["late-task"]
	s.mu.Unlock()
	if !queued {
		t.Fatal("late-task was not queued")
	}
	if ScanDroppedCount(ScanDroppedQueueFull) != dropped {
		t.Fatal("waiting for room counted a drop")
	}
}

func TestScanner_DailyPassWaitEndsOnStop(t *testing.T) {
	f := moveBackFixture(t)
	s := f.scanner(&fakeChecker{})
	fillScanQueue(s)
	done := make(chan bool, 1)
	go func() { done <- s.enqueueWaiting(context.Background(), "late-task") }()
	s.Stop()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("enqueueWaiting = true after Stop")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("enqueueWaiting outlived Stop")
	}
}
