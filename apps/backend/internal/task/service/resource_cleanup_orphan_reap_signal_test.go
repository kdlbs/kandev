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
	onAlive   func(pid int)
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
	alive, ok := f.alive[pid]
	hook := f.onAlive
	f.mu.Unlock()
	if hook != nil {
		hook(pid)
	}
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

// AC-TASKS-ORPHAN-REAP-004.6 + REQ-005: an indeterminate liveness read (the
// platform could not tell whether the process is still alive) must never be
// recorded as a successful reap. It is treated the same as "still alive":
// escalation proceeds to SIGKILL, and if liveness is still indeterminate
// afterward, the candidate is recorded survived with a retryable error --
// never terminated/killed on a liveness check that was never actually
// confirmed.
func TestSignalOrphanReapCandidatesTreatsUnknownLivenessAsUnresolvedNotTerminated(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newOrphanReapSignalTestService()
		verifier := newFakeOrphanReapVerifier()
		verifier.set(500, "/tasks/task-a")
		signaler := newFakeOrphanReapSignaler()
		// Deliberately never call setAlive: Alive(500) returns (false, false)
		// throughout, simulating a platform liveness probe that can never
		// determine the process's state.
		svc.orphanReapVerifier = verifier
		svc.orphanReapSignaler = signaler

		cand := newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a")
		snapshot := &taskResourceCleanupSnapshot{}
		errs := svc.signalOrphanReapCandidates(context.Background(), "task-a", []orphanReapCandidate{cand}, snapshot)
		if len(errs) != 1 || !errors.Is(errs[0], errOrphanReapCandidateSurvived) {
			t.Fatalf("expected errOrphanReapCandidateSurvived on unresolved liveness, got %v", errs)
		}
		rec, ok := findOrphanReapRecord(snapshot, 500)
		if !ok || rec.Outcome != orphanReapOutcomeSurvived {
			t.Fatalf("expected pid 500 recorded survived (never confirmed dead), got %+v (found=%v)", rec, ok)
		}
		sent := signaler.sentSignals()
		if len(sent) != 2 || sent[0].sig != orphanReapSigterm || sent[1].sig != orphanReapSigkill {
			t.Fatalf("expected escalation to proceed through sigterm and sigkill despite unresolved "+
				"liveness (never short-circuited to a false 'terminated'), got %+v", sent)
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

// AC-TASKS-ORPHAN-REAP-003.7: the pre-SIGTERM re-verification -- the check
// standing between a reused PID and the first irreversible signal -- must
// itself be able to fail and stop that signal, not just the pre-SIGKILL one.
func TestSignalOrphanReapCandidatesReverifiesBeforeSigterm(t *testing.T) {
	verifier := newFakeOrphanReapVerifier()
	verifier.set(500, "/unrelated/dir") // already outside the root at enumeration-to-signal time
	signaler := newFakeOrphanReapSignaler()
	signaler.setAlive(500, true)

	svc := newOrphanReapSignalTestService()
	svc.orphanReapVerifier = verifier
	svc.orphanReapSignaler = signaler

	cand := newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a")
	snapshot := &taskResourceCleanupSnapshot{}
	errs := svc.signalOrphanReapCandidates(context.Background(), "task-a", []orphanReapCandidate{cand}, snapshot)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if sent := signaler.sentSignals(); len(sent) != 0 {
		t.Fatalf("expected no SIGTERM sent once re-verification found the pid outside its root, got %+v", sent)
	}
	rec, ok := findOrphanReapRecord(snapshot, 500)
	if !ok || rec.Outcome != orphanReapOutcomeSkipped {
		t.Fatalf("expected pid 500 recorded skipped before any signal, got %+v (found=%v)", rec, ok)
	}
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
		// AC-TASKS-ORPHAN-REAP-006.3: pid 600 already received SIGTERM before
		// this cancellation, so it must still be persisted as skipped naming
		// that signal, not silently dropped for never reaching the SIGKILL
		// check.
		rec, ok := findOrphanReapRecord(snapshot, 600)
		if !ok || rec.Outcome != orphanReapOutcomeSkipped || rec.Reason != orphanReapLastSignalSigtermSent {
			t.Fatalf("expected pid 600 (already sigtermed, never reached by the sigkill loop) recorded "+
				"skipped/\"sigterm already sent\", got %+v (found=%v)", rec, ok)
		}
	})
}

// cancelDuringVerify wraps a verifier and cancels the context as a side
// effect of a successful VerifyCwd, simulating cancellation landing during a
// context-blind verifier's read (Linux's VerifyCwd is a bare os.Readlink that
// never observes ctx) -- after the per-iteration ctx.Err() check at the top
// of the send loop already passed, but before the loop would otherwise reach
// the signal send that follows a successful reverify.
type cancelDuringVerify struct {
	inner  orphanReapVerifier
	cancel context.CancelFunc
}

func (v cancelDuringVerify) VerifyCwd(ctx context.Context, pid int) (string, error) {
	cwd, err := v.inner.VerifyCwd(ctx, pid)
	v.cancel()
	return cwd, err
}

// AC-TASKS-ORPHAN-REAP-006.3: cancellation landing during the mandatory
// re-verification itself -- after the loop-top ctx.Err() check already
// passed -- must still stop the SIGTERM that would otherwise immediately
// follow a successful reverify.
func TestSignalOrphanReapCandidatesRechecksCancellationAfterReverifyBeforeSigterm(t *testing.T) {
	verifier := newFakeOrphanReapVerifier()
	verifier.set(500, "/tasks/task-a")
	signaler := newFakeOrphanReapSignaler()
	signaler.setAlive(500, true)

	svc := newOrphanReapSignalTestService()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.orphanReapVerifier = cancelDuringVerify{inner: verifier, cancel: cancel}
	svc.orphanReapSignaler = signaler

	candidates := []orphanReapCandidate{
		newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a"),
	}
	snapshot := &taskResourceCleanupSnapshot{}
	errs := svc.signalOrphanReapCandidates(ctx, "task-a", candidates, snapshot)

	if len(errs) != 1 || !errors.Is(errs[0], errOrphanReapCancelledMidPhase) {
		t.Fatalf("expected errOrphanReapCancelledMidPhase, got %v", errs)
	}
	if sent := signaler.sentSignals(); len(sent) != 0 {
		t.Fatalf("expected no SIGTERM sent once cancellation lands during reverify, got %+v", sent)
	}
}

// The same recheck applies to the SIGKILL loop, between its reverify and the
// SIGKILL send.
func TestSignalOrphanReapCandidatesRechecksCancellationAfterReverifyBeforeSigkill(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		verifier := newFakeOrphanReapVerifier()
		verifier.set(500, "/tasks/task-a")
		signaler := newFakeOrphanReapSignaler()
		signaler.setAlive(500, true) // survives sigterm, escalates to sigkill

		svc := newOrphanReapSignalTestService()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		cancelOnSigkillReverify := false
		svc.orphanReapVerifier = verifierFunc(func(vctx context.Context, pid int) (string, error) {
			cwd, err := verifier.VerifyCwd(vctx, pid)
			if cancelOnSigkillReverify {
				cancel()
			}
			return cwd, err
		})
		signaler.onSignal = func(pid int, sig orphanReapSignal) {
			if sig == orphanReapSigterm {
				cancelOnSigkillReverify = true // arm cancellation for the SIGKILL-phase reverify only
			}
		}
		svc.orphanReapSignaler = signaler

		candidates := []orphanReapCandidate{
			newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a"),
		}
		snapshot := &taskResourceCleanupSnapshot{}
		errs := svc.signalOrphanReapCandidates(ctx, "task-a", candidates, snapshot)

		if len(errs) != 1 || !errors.Is(errs[0], errOrphanReapCancelledMidPhase) {
			t.Fatalf("expected errOrphanReapCancelledMidPhase, got %v", errs)
		}
		for _, s := range signaler.sentSignals() {
			if s.sig == orphanReapSigkill {
				t.Fatalf("expected no SIGKILL sent once cancellation lands during reverify, got %+v", signaler.sentSignals())
			}
		}
	})
}

// verifierFunc adapts a plain function to orphanReapVerifier.
type verifierFunc func(ctx context.Context, pid int) (string, error)

func (f verifierFunc) VerifyCwd(ctx context.Context, pid int) (string, error) { return f(ctx, pid) }

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

// AC-TASKS-ORPHAN-REAP-006.3: cancellation landing after the SIGKILL settle
// window -- a race the settle-delay select cannot always resolve in the
// cancellation's favor, since both its channels can already be ready --
// must still persist every already-signalled candidate as skipped naming
// the signal already sent, and return the retryable cancellation error,
// instead of resolving them to a normal killed/survived outcome.
func TestResolveOrphanReapSurvivorsRecordsCancelledWhenContextAlreadyDone(t *testing.T) {
	svc := newOrphanReapSignalTestService()
	verifier := newFakeOrphanReapVerifier()
	verifier.set(500, "/tasks/task-a")
	signaler := newFakeOrphanReapSignaler()
	signaler.setAlive(500, false) // would otherwise resolve cleanly to "killed"

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	killPending := []orphanReapPendingCandidate{{
		orphanReapCandidate: newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a"),
		lastSignal:          orphanReapLastSignalSigkillSent,
	}}
	snapshot := &taskResourceCleanupSnapshot{}
	errs := svc.resolveOrphanReapSurvivors(ctx, "task-a", killPending, snapshot, verifier, signaler)

	if len(errs) != 1 || !errors.Is(errs[0], errOrphanReapCancelledMidPhase) {
		t.Fatalf("expected errOrphanReapCancelledMidPhase, got %v", errs)
	}
	rec, ok := findOrphanReapRecord(snapshot, 500)
	if !ok || rec.Outcome != orphanReapOutcomeSkipped || rec.Reason != orphanReapLastSignalSigkillSent {
		t.Fatalf("expected pid 500 recorded skipped/\"sigkill already sent\" despite cancellation, "+
			"not resolved to a normal outcome, got %+v (found=%v)", rec, ok)
	}
}

// AC-TASKS-ORPHAN-REAP-006.3: cancellation landing between candidates in this
// loop -- after the first is already classified, before the second is --
// must still persist the not-yet-classified remainder as skipped naming the
// signal already sent, without disturbing the first candidate's accurate
// outcome.
func TestResolveOrphanReapSurvivorsStopsOnCancellationMidLoop(t *testing.T) {
	svc := newOrphanReapSignalTestService()
	verifier := newFakeOrphanReapVerifier()
	verifier.set(600, "/tasks/task-a")
	signaler := newFakeOrphanReapSignaler()
	signaler.setAlive(500, false) // confirmed dead
	signaler.setAlive(600, true)  // would otherwise resolve cleanly to "survived"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signaler.onAlive = func(pid int) {
		if pid == 500 {
			cancel()
		}
	}

	killPending := []orphanReapPendingCandidate{
		{orphanReapCandidate: newOrphanReapOwnershipCandidate(500, 1, "/tasks/task-a", "/tasks/task-a"), lastSignal: orphanReapLastSignalSigkillSent},
		{orphanReapCandidate: newOrphanReapOwnershipCandidate(600, 1, "/tasks/task-a", "/tasks/task-a"), lastSignal: orphanReapLastSignalSigkillSent},
	}
	snapshot := &taskResourceCleanupSnapshot{}
	errs := svc.resolveOrphanReapSurvivors(ctx, "task-a", killPending, snapshot, verifier, signaler)

	if len(errs) != 1 || !errors.Is(errs[0], errOrphanReapCancelledMidPhase) {
		t.Fatalf("expected errOrphanReapCancelledMidPhase, got %v", errs)
	}
	rec500, ok := findOrphanReapRecord(snapshot, 500)
	if !ok || rec500.Outcome != orphanReapOutcomeKilled {
		t.Fatalf("expected pid 500 (classified before cancellation) to keep its accurate killed outcome, "+
			"got %+v (found=%v)", rec500, ok)
	}
	rec600, ok := findOrphanReapRecord(snapshot, 600)
	if !ok || rec600.Outcome != orphanReapOutcomeSkipped || rec600.Reason != orphanReapLastSignalSigkillSent {
		t.Fatalf("expected pid 600 (not yet classified when cancellation landed) recorded "+
			"skipped/\"sigkill already sent\", not resolved to survived, got %+v (found=%v)", rec600, ok)
	}
}
