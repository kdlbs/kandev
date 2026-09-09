package service

import (
	"context"
	"errors"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"

	"github.com/kandev/kandev/internal/common/logger"
)

// fakeOrphanReapVerifier answers VerifyCwd from an in-memory pid->cwd map so
// tests never read a real process's filesystem state. A pid absent from cwd
// simulates the process being gone or unreadable.
type fakeOrphanReapVerifier struct {
	mu  sync.Mutex
	cwd map[int]string
}

func newFakeOrphanReapVerifier() *fakeOrphanReapVerifier {
	return &fakeOrphanReapVerifier{cwd: map[int]string{}}
}

func (f *fakeOrphanReapVerifier) set(pid int, cwd string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cwd[pid] = cwd
}

func (f *fakeOrphanReapVerifier) VerifyCwd(_ context.Context, pid int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cwd, ok := f.cwd[pid]
	if !ok {
		return "", errors.New("fake verifier: pid not found")
	}
	return cwd, nil
}

// fakeOrphanReapSignaler never issues a real syscall: this feature signals
// processes Kandev does not own, so a test must not risk hitting an arbitrary
// real host PID. Alive state and per-pid errors are entirely in-memory.
type fakeOrphanReapSignaler struct {
	mu        sync.Mutex
	alive     map[int]bool
	signalErr map[int]error
	sent      []fakeOrphanReapSentSignal
	onSignal  func(pid int, sig orphanReapSignal)
}

type fakeOrphanReapSentSignal struct {
	pid int
	sig orphanReapSignal
}

func newFakeOrphanReapSignaler() *fakeOrphanReapSignaler {
	return &fakeOrphanReapSignaler{alive: map[int]bool{}, signalErr: map[int]error{}}
}

func (f *fakeOrphanReapSignaler) setAlive(pid int, alive bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alive[pid] = alive
}

func (f *fakeOrphanReapSignaler) sentSignals() []fakeOrphanReapSentSignal {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeOrphanReapSentSignal, len(f.sent))
	copy(out, f.sent)
	return out
}

func (f *fakeOrphanReapSignaler) Signal(pid int, sig orphanReapSignal) error {
	f.mu.Lock()
	f.sent = append(f.sent, fakeOrphanReapSentSignal{pid: pid, sig: sig})
	err, hasErr := f.signalErr[pid]
	hook := f.onSignal
	f.mu.Unlock()
	if hook != nil {
		hook(pid, sig)
	}
	if hasErr {
		return err
	}
	return nil
}

func (f *fakeOrphanReapSignaler) Alive(pid int) (alive, known bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	alive, ok := f.alive[pid]
	return alive, ok
}

func newOrphanReapSignalTestService() *Service {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	return &Service{logger: log}
}

func findOrphanReapRecord(snapshot *taskResourceCleanupSnapshot, pid int) (orphanReapCandidateRecord, bool) {
	for _, rec := range snapshot.OrphanReapRecords {
		if rec.PID == pid {
			return rec, true
		}
	}
	return orphanReapCandidateRecord{}, false
}

// A candidate that dies between SIGTERM and the SIGKILL check never needs
// SIGKILL and is recorded terminated (AC-TASKS-ORPHAN-REAP-004.2/004.4).
func TestSignalOrphanReapCandidatesTerminatesOnSigterm(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newOrphanReapSignalTestService()
		verifier := newFakeOrphanReapVerifier()
		verifier.set(500, "/tasks/task-a")
		signaler := newFakeOrphanReapSignaler()
		signaler.setAlive(500, true)
		signaler.onSignal = func(pid int, sig orphanReapSignal) {
			if sig == orphanReapSigterm {
				signaler.setAlive(pid, false)
			}
		}
		svc.orphanReapVerifier = verifier
		svc.orphanReapSignaler = signaler

		cand := newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a")
		snapshot := &taskResourceCleanupSnapshot{}
		errs := svc.signalOrphanReapCandidates(context.Background(), "task-a", []orphanReapCandidate{cand}, snapshot)
		if len(errs) != 0 {
			t.Fatalf("expected no errors, got %v", errs)
		}
		rec, ok := findOrphanReapRecord(snapshot, 500)
		if !ok || rec.Outcome != orphanReapOutcomeTerminated {
			t.Fatalf("expected pid 500 recorded terminated, got %+v (found=%v)", rec, ok)
		}
		for _, s := range signaler.sentSignals() {
			if s.sig == orphanReapSigkill {
				t.Fatalf("expected no SIGKILL once SIGTERM already terminated the process, got %+v", signaler.sentSignals())
			}
		}
	})
}

// A candidate still alive after the grace period escalates to SIGKILL
// (AC-TASKS-ORPHAN-REAP-004.2) and is recorded killed once it dies.
func TestSignalOrphanReapCandidatesEscalatesToSigkill(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newOrphanReapSignalTestService()
		verifier := newFakeOrphanReapVerifier()
		verifier.set(500, "/tasks/task-a")
		signaler := newFakeOrphanReapSignaler()
		signaler.setAlive(500, true)
		signaler.onSignal = func(pid int, sig orphanReapSignal) {
			if sig == orphanReapSigkill {
				signaler.setAlive(pid, false)
			}
		}
		svc.orphanReapVerifier = verifier
		svc.orphanReapSignaler = signaler

		cand := newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a")
		snapshot := &taskResourceCleanupSnapshot{}
		errs := svc.signalOrphanReapCandidates(context.Background(), "task-a", []orphanReapCandidate{cand}, snapshot)
		if len(errs) != 0 {
			t.Fatalf("expected no errors, got %v", errs)
		}
		rec, ok := findOrphanReapRecord(snapshot, 500)
		if !ok || rec.Outcome != orphanReapOutcomeKilled {
			t.Fatalf("expected pid 500 recorded killed, got %+v (found=%v)", rec, ok)
		}
		sent := signaler.sentSignals()
		if len(sent) != 2 || sent[0].sig != orphanReapSigterm || sent[1].sig != orphanReapSigkill {
			t.Fatalf("expected sigterm then sigkill, got %+v", sent)
		}
	})
}

// A candidate still alive after the SIGKILL settle window is recorded
// survived and produces a retryable error (AC-TASKS-ORPHAN-REAP-004.6).
func TestSignalOrphanReapCandidatesSurvivesSigkill(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newOrphanReapSignalTestService()
		verifier := newFakeOrphanReapVerifier()
		verifier.set(500, "/tasks/task-a")
		signaler := newFakeOrphanReapSignaler()
		signaler.setAlive(500, true) // never dies
		svc.orphanReapVerifier = verifier
		svc.orphanReapSignaler = signaler

		cand := newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a")
		snapshot := &taskResourceCleanupSnapshot{}
		errs := svc.signalOrphanReapCandidates(context.Background(), "task-a", []orphanReapCandidate{cand}, snapshot)
		if len(errs) != 1 || !errors.Is(errs[0], errOrphanReapCandidateSurvived) {
			t.Fatalf("expected errOrphanReapCandidateSurvived, got %v", errs)
		}
		rec, ok := findOrphanReapRecord(snapshot, 500)
		if !ok || rec.Outcome != orphanReapOutcomeSurvived {
			t.Fatalf("expected pid 500 recorded survived, got %+v (found=%v)", rec, ok)
		}
	})
}

// AC-TASKS-ORPHAN-REAP-003.7: identity is re-verified immediately before
// every signal. A pid whose cwd has moved outside its root between attribution
// and the SIGKILL decision must not receive SIGKILL.
func TestSignalOrphanReapCandidatesReverifiesBeforeSigkill(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newOrphanReapSignalTestService()
		verifier := newFakeOrphanReapVerifier()
		verifier.set(500, "/tasks/task-a") // inside root at sigterm time
		signaler := newFakeOrphanReapSignaler()
		signaler.setAlive(500, true)
		signaler.onSignal = func(pid int, sig orphanReapSignal) {
			if sig == orphanReapSigterm {
				// The pid is reused by an unrelated process outside the root
				// before the grace period ends.
				verifier.set(pid, "/unrelated/dir")
			}
		}
		svc.orphanReapVerifier = verifier
		svc.orphanReapSignaler = signaler

		cand := newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a")
		snapshot := &taskResourceCleanupSnapshot{}
		errs := svc.signalOrphanReapCandidates(context.Background(), "task-a", []orphanReapCandidate{cand}, snapshot)
		if len(errs) != 0 {
			t.Fatalf("expected no errors, got %v", errs)
		}
		rec, ok := findOrphanReapRecord(snapshot, 500)
		if !ok || rec.Outcome != orphanReapOutcomeSkipped {
			t.Fatalf("expected pid 500 recorded skipped after re-verification failed, got %+v (found=%v)", rec, ok)
		}
		for _, s := range signaler.sentSignals() {
			if s.sig == orphanReapSigkill {
				t.Fatalf("expected no SIGKILL sent once re-verification moved the pid outside its root, got %+v", signaler.sentSignals())
			}
		}
	})
}

// AC-TASKS-ORPHAN-REAP-004.3: every SIGTERM is sent before the shared grace
// period starts, regardless of how many candidates there are.
func TestSignalOrphanReapCandidatesSendsAllSigtermsBeforeGraceDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newOrphanReapSignalTestService()
		verifier := newFakeOrphanReapVerifier()
		verifier.set(500, "/tasks/task-a")
		verifier.set(600, "/tasks/task-a")
		signaler := newFakeOrphanReapSignaler()
		signaler.setAlive(500, true)
		signaler.setAlive(600, true)
		svc.orphanReapVerifier = verifier
		svc.orphanReapSignaler = signaler

		candidates := []orphanReapCandidate{
			newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a"),
			newOrphanReapOwnershipCandidate(600, 1, "/tasks/task-a", "/tasks/task-a"),
		}
		snapshot := &taskResourceCleanupSnapshot{}
		errs := svc.signalOrphanReapCandidates(context.Background(), "task-a", candidates, snapshot)
		if len(errs) != 1 || !errors.Is(errs[0], errOrphanReapCandidateSurvived) {
			t.Fatalf("expected both survivors, got errs=%v", errs)
		}
		sent := signaler.sentSignals()
		if len(sent) != 4 {
			t.Fatalf("expected 2 sigterms + 2 sigkills, got %+v", sent)
		}
		if sent[0].sig != orphanReapSigterm || sent[1].sig != orphanReapSigterm {
			t.Fatalf("expected both sigterms sent before any sigkill, got %+v", sent)
		}
	})
}

// AC-TASKS-ORPHAN-REAP-006.3: cancellation after a signal has already been
// sent stops further escalation and records the pending candidate skipped,
// never silently dropped.
func TestSignalOrphanReapCandidatesCancelledMidPhaseAfterSigterm(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newOrphanReapSignalTestService()
		verifier := newFakeOrphanReapVerifier()
		verifier.set(500, "/tasks/task-a")
		signaler := newFakeOrphanReapSignaler()
		signaler.setAlive(500, true) // never dies; cancellation must still stop escalation
		svc.orphanReapVerifier = verifier
		svc.orphanReapSignaler = signaler

		ctx, cancel := context.WithCancel(context.Background())
		cand := newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a")
		snapshot := &taskResourceCleanupSnapshot{}

		done := make(chan []error, 1)
		go func() { done <- svc.signalOrphanReapCandidates(ctx, "task-a", []orphanReapCandidate{cand}, snapshot) }()
		synctest.Wait()
		cancel()
		errs := <-done

		if len(errs) != 1 || !errors.Is(errs[0], errOrphanReapCancelledMidPhase) {
			t.Fatalf("expected errOrphanReapCancelledMidPhase, got %v", errs)
		}
		rec, ok := findOrphanReapRecord(snapshot, 500)
		if !ok || rec.Outcome != orphanReapOutcomeSkipped {
			t.Fatalf("expected pid 500 recorded skipped after cancellation, got %+v (found=%v)", rec, ok)
		}
		for _, s := range signaler.sentSignals() {
			if s.sig == orphanReapSigkill {
				t.Fatalf("expected no SIGKILL sent after cancellation stopped escalation, got %+v", signaler.sentSignals())
			}
		}
	})
}

// Cancellation arriving mid-loop over multiple candidates must stop
// sending further signals immediately, not just at the next checkpoint. The
// fake verifier here is context-blind (VerifyCwd ignores its ctx parameter),
// matching real Linux behavior — the loop itself, not the verifier, must be
// what stops signalling the remaining candidates.
func TestSignalOrphanReapCandidatesStopsLoopOnCancellationMidBurst(t *testing.T) {
	verifier := newFakeOrphanReapVerifier()
	verifier.set(500, "/tasks/task-a")
	verifier.set(600, "/tasks/task-a")
	verifier.set(700, "/tasks/task-a")
	signaler := newFakeOrphanReapSignaler()
	signaler.setAlive(500, true)
	signaler.setAlive(600, true)
	signaler.setAlive(700, true)

	svc := newOrphanReapSignalTestService()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signaler.onSignal = func(pid int, sig orphanReapSignal) {
		if pid == 500 && sig == orphanReapSigterm {
			cancel()
		}
	}
	svc.orphanReapVerifier = verifier
	svc.orphanReapSignaler = signaler

	candidates := []orphanReapCandidate{
		newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a"),
		newOrphanReapOwnershipCandidate(600, 1, "/tasks/task-a", "/tasks/task-a"),
		newOrphanReapOwnershipCandidate(700, 1, "/tasks/task-a", "/tasks/task-a"),
	}
	snapshot := &taskResourceCleanupSnapshot{}
	errs := svc.signalOrphanReapCandidates(ctx, "task-a", candidates, snapshot)

	if len(errs) != 1 || !errors.Is(errs[0], errOrphanReapCancelledMidPhase) {
		t.Fatalf("expected errOrphanReapCancelledMidPhase, got %v", errs)
	}
	sent := signaler.sentSignals()
	if len(sent) != 1 || sent[0].pid != 500 {
		t.Fatalf("expected only pid 500 to have been signalled before cancellation stopped the loop, got %+v", sent)
	}
	if _, ok := findOrphanReapRecord(snapshot, 600); ok {
		t.Fatalf("expected pid 600 (never reached) to have no record")
	}
	if _, ok := findOrphanReapRecord(snapshot, 700); ok {
		t.Fatalf("expected pid 700 (never reached) to have no record")
	}
	rec, ok := findOrphanReapRecord(snapshot, 500)
	if !ok || rec.Outcome != orphanReapOutcomeSkipped {
		t.Fatalf("expected pid 500 (already signalled) recorded skipped, got %+v (found=%v)", rec, ok)
	}
}

// The same mid-burst stop applies to the SIGKILL loop.
func TestSignalOrphanReapCandidatesStopsSigkillLoopOnCancellationMidBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		verifier := newFakeOrphanReapVerifier()
		verifier.set(500, "/tasks/task-a")
		verifier.set(600, "/tasks/task-a")
		signaler := newFakeOrphanReapSignaler()
		signaler.setAlive(500, true)
		signaler.setAlive(600, true) // both survive sigterm, escalate to sigkill

		svc := newOrphanReapSignalTestService()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		signaler.onSignal = func(pid int, sig orphanReapSignal) {
			if pid == 500 && sig == orphanReapSigkill {
				cancel()
			}
		}
		svc.orphanReapVerifier = verifier
		svc.orphanReapSignaler = signaler

		candidates := []orphanReapCandidate{
			newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a"),
			newOrphanReapOwnershipCandidate(600, 1, "/tasks/task-a", "/tasks/task-a"),
		}
		snapshot := &taskResourceCleanupSnapshot{}
		errs := svc.signalOrphanReapCandidates(ctx, "task-a", candidates, snapshot)

		if len(errs) != 1 || !errors.Is(errs[0], errOrphanReapCancelledMidPhase) {
			t.Fatalf("expected errOrphanReapCancelledMidPhase, got %v", errs)
		}
		for _, s := range signaler.sentSignals() {
			if s.pid == 600 && s.sig == orphanReapSigkill {
				t.Fatalf("expected no SIGKILL sent to pid 600 after cancellation stopped the loop, got %+v", signaler.sentSignals())
			}
		}
	})
}

// AC-TASKS-ORPHAN-REAP-004.4: a signal failing because the process is already
// gone is a successful reap (terminated), not an error.
func TestRecordOrphanReapSignalErrorProcessGoneIsTerminated(t *testing.T) {
	svc := newOrphanReapSignalTestService()
	cand := newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a")
	snapshot := &taskResourceCleanupSnapshot{}
	svc.recordOrphanReapSignalError(snapshot, "task-a", cand, "sigterm", syscall.ESRCH)
	rec, ok := findOrphanReapRecord(snapshot, 500)
	if !ok || rec.Outcome != orphanReapOutcomeTerminated {
		t.Fatalf("expected ESRCH to record terminated, got %+v (found=%v)", rec, ok)
	}
}

// AC-TASKS-ORPHAN-REAP-004.5: permission denied is a non-retryable skip.
func TestRecordOrphanReapSignalErrorPermissionDeniedIsSkipped(t *testing.T) {
	svc := newOrphanReapSignalTestService()
	cand := newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a")
	snapshot := &taskResourceCleanupSnapshot{}
	svc.recordOrphanReapSignalError(snapshot, "task-a", cand, "sigterm", syscall.EPERM)
	rec, ok := findOrphanReapRecord(snapshot, 500)
	if !ok || rec.Outcome != orphanReapOutcomeSkipped || rec.Reason != "permission denied" {
		t.Fatalf("expected EPERM to record skipped/permission denied, got %+v (found=%v)", rec, ok)
	}
}

// Any other signal failure is treated conservatively as a skip so a
// transient OS error never masquerades as a successful reap.
func TestRecordOrphanReapSignalErrorOtherIsSkippedWithMessage(t *testing.T) {
	svc := newOrphanReapSignalTestService()
	cand := newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a")
	snapshot := &taskResourceCleanupSnapshot{}
	svc.recordOrphanReapSignalError(snapshot, "task-a", cand, "sigterm", errors.New("transient failure"))
	rec, ok := findOrphanReapRecord(snapshot, 500)
	if !ok || rec.Outcome != orphanReapOutcomeSkipped || rec.Reason != "sigterm failed: transient failure" {
		t.Fatalf("expected a skipped record naming the failure, got %+v (found=%v)", rec, ok)
	}
}
