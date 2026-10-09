package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"go.uber.org/zap"
)

type fakeAdmittedLister struct {
	mu  sync.Mutex
	ids []string
	err error
}

func (f *fakeAdmittedLister) ListAdmittedSessionIDs(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return append([]string(nil), f.ids...), nil
}

func (f *fakeAdmittedLister) set(ids ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ids = ids
}

func (f *fakeAdmittedLister) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func newTestController(t *testing.T, ceiling int, lister *fakeAdmittedLister) *sessionCeilingController {
	t.Helper()
	c := newSessionCeilingController(ceiling, lister, zap.NewNop())
	if c == nil {
		t.Fatal("newSessionCeilingController returned nil")
	}
	return c
}

func mustPopulation(t *testing.T, c *sessionCeilingController) int {
	t.Helper()
	got, err := c.population(context.Background())
	if err != nil {
		t.Fatalf("population: %v", err)
	}
	return got
}

// The persisted rows alone are the population when nothing is in flight.
func TestPopulationCountsPersistedRows(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1", "s2", "s3")
	c := newTestController(t, 10, lister)
	if got := mustPopulation(t, c); got != 3 {
		t.Fatalf("population = %d, want 3", got)
	}
}

// A session holding both a row and a reservation is one session, not two. Counting
// it twice would refuse a launch the instance has room for.
func TestPopulationUnionsRowsWithReservationsBySessionID(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1", "s2")
	c := newTestController(t, 10, lister)

	d := c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s9", origin: launchOriginAutomatic, seam: "seam2"})
	if !d.admitted || d.reservationKey != "s9" {
		t.Fatalf("admit = %+v, want admitted holding a reservation for s9", d)
	}
	if got := mustPopulation(t, c); got != 3 {
		t.Fatalf("population = %d, want 3 (two rows plus one reservation)", got)
	}

	// s9's launch reaches STARTING: it now holds a row AND its reservation, and it
	// is still one session. Counting both would refuse a launch there is room for.
	lister.set("s1", "s2", "s9")
	if got := mustPopulation(t, c); got != 3 {
		t.Fatalf("population = %d, want 3 (s9's row and reservation are one session)", got)
	}
}

// A seam-1 reservation has no session id yet and can collide with no row, so it
// adds to the population. Omitting it would make every seam-1 admission invisible
// to the ceiling until its session row lands.
func TestPopulationCountsLaunchScopedReservations(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1")
	c := newTestController(t, 10, lister)

	d := c.admit(context.Background(), admissionRequest{taskID: "t1", origin: launchOriginAutomatic, seam: "seam1"})
	if !d.admitted {
		t.Fatalf("admit refused below the ceiling: %+v", d)
	}
	if d.reservationKey == "" {
		t.Fatal("admit returned no reservation key for a seam-1 launch")
	}
	if got := mustPopulation(t, c); got != 2 {
		t.Fatalf("population = %d, want 2 (one row plus one launch-scoped reservation)", got)
	}
}

func TestAdmitBelowCeilingReservesCapacity(t *testing.T) {
	lister := &fakeAdmittedLister{}
	c := newTestController(t, 2, lister)

	d := c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s1", origin: launchOriginAutomatic, seam: "seam2"})
	if !d.admitted || d.manualOverride {
		t.Fatalf("admit = %+v, want admitted without override", d)
	}
	if d.reservationKey != "s1" {
		t.Fatalf("reservationKey = %q, want %q", d.reservationKey, "s1")
	}
	if d.ceiling != 2 || d.population != 0 || !d.populationKnown {
		t.Fatalf("decision reported population %d/%d (known=%v), want 0/2 known", d.population, d.ceiling, d.populationKnown)
	}
	if got := mustPopulation(t, c); got != 1 {
		t.Fatalf("population after admit = %d, want 1", got)
	}
}

// A disabled ceiling must not make automatic admission depend on the
// population reader. The reader can be unavailable during startup or a
// transient database failure, but an unlimited controller still needs to keep
// the launch reservation for its later lifecycle callback.
func TestDisabledCeilingAdmitsWhenPopulationReadFails(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.fail(errors.New("population unavailable"))
	c := newTestController(t, unlimitedSessionCeiling, lister)

	decision := c.admit(context.Background(), admissionRequest{
		taskID: "unlimited-task", sessionID: "unlimited-session",
		origin: launchOriginAutomatic, seam: "disabled-test",
	})
	if !decision.admitted {
		t.Fatalf("disabled ceiling refused an automatic launch: %+v", decision)
	}
	if decision.manualOverride || decision.reasonCode != "" {
		t.Fatalf("disabled ceiling recorded an override/reason: %+v", decision)
	}
	if decision.reservationKey != "unlimited-session" {
		t.Fatalf("reservation key = %q, want session reservation", decision.reservationKey)
	}
}

func TestAdmitAtCeilingRefusesAutomaticLaunch(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1", "s2")
	c := newTestController(t, 2, lister)

	d := c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s9", origin: launchOriginAutomatic, seam: "seam2"})
	if d.admitted {
		t.Fatalf("admit = %+v, want refused at the ceiling", d)
	}
	if d.reasonCode != ceilingReasonRefused {
		t.Fatalf("reasonCode = %q, want %q", d.reasonCode, ceilingReasonRefused)
	}
	if d.reservationKey != "" {
		t.Fatalf("a refusal took reservation %q, want none", d.reservationKey)
	}
	if got := mustPopulation(t, c); got != 2 {
		t.Fatalf("population after a refusal = %d, want 2 unchanged", got)
	}
}

// A manual launch is admitted over the ceiling and still counts, so the population
// rises above the ceiling and subsequent automatic launches stay refused.
func TestAdmitAtCeilingAdmitsManualLaunchAndCountsIt(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1", "s2")
	c := newTestController(t, 2, lister)

	d := c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s9", origin: launchOriginManual, seam: "seam2"})
	if !d.admitted || !d.manualOverride {
		t.Fatalf("admit = %+v, want admitted as a manual override", d)
	}
	if d.reasonCode != ceilingReasonManualOverride {
		t.Fatalf("reasonCode = %q, want %q", d.reasonCode, ceilingReasonManualOverride)
	}
	if got := mustPopulation(t, c); got != 3 {
		t.Fatalf("population after a manual override = %d, want 3 (above the ceiling)", got)
	}
	next := c.admit(context.Background(), admissionRequest{taskID: "t2", sessionID: "s10", origin: launchOriginAutomatic, seam: "seam2"})
	if next.admitted {
		t.Fatalf("automatic launch admitted while the population is above the ceiling: %+v", next)
	}
}

// A second request for a session that already holds a reservation consumes no
// second unit: one launch is capped at one unit of capacity however many seams it
// passes through.
func TestAdmitIsIdempotentPerSessionID(t *testing.T) {
	lister := &fakeAdmittedLister{}
	c := newTestController(t, 2, lister)

	first := c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s1", origin: launchOriginAutomatic, seam: "seam5"})
	second := c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s1", origin: launchOriginAutomatic, seam: "seam2"})
	if !first.admitted || !second.admitted {
		t.Fatalf("first = %+v, second = %+v, want both admitted", first, second)
	}
	if got := mustPopulation(t, c); got != 1 {
		t.Fatalf("population after two admissions of one session = %d, want 1", got)
	}
}

// A session already counted as a row is admitted without taking a reservation, so a
// duplicate retry of a running launch cannot consume capacity.
func TestAdmitShortCircuitsForAnAlreadyCountedSession(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1", "s2")
	c := newTestController(t, 2, lister)

	d := c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s1", origin: launchOriginAutomatic, seam: "seam3"})
	if !d.admitted {
		t.Fatalf("admit = %+v, want admitted for an already-counted session even at the ceiling", d)
	}
	if got := mustPopulation(t, c); got != 2 {
		t.Fatalf("population = %d, want 2 unchanged", got)
	}
}

func TestReleaseIsIdempotentAndIgnoresUnknownKeys(t *testing.T) {
	lister := &fakeAdmittedLister{}
	c := newTestController(t, 4, lister)
	c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s1", origin: launchOriginAutomatic, seam: "seam2"})

	c.release("does-not-exist")
	if got := mustPopulation(t, c); got != 1 {
		t.Fatalf("population after releasing an unknown key = %d, want 1", got)
	}
	c.release("s1")
	c.release("s1")
	if got := mustPopulation(t, c); got != 0 {
		t.Fatalf("population after a double release = %d, want 0", got)
	}
}

// Zero is unlimited, matching the wip_limit convention.
func TestUnlimitedCeilingAdmitsEverything(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1", "s2", "s3", "s4", "s5")
	c := newTestController(t, unlimitedSessionCeiling, lister)

	for i := 0; i < 20; i++ {
		d := c.admit(context.Background(), admissionRequest{taskID: "t", sessionID: fmt.Sprintf("new-%d", i), origin: launchOriginAutomatic, seam: "seam2"})
		if !d.admitted || d.manualOverride {
			t.Fatalf("admit %d = %+v, want plain admitted under an unlimited ceiling", i, d)
		}
	}
}

func TestCeilingOfOneAdmitsExactlyOneSession(t *testing.T) {
	lister := &fakeAdmittedLister{}
	c := newTestController(t, 1, lister)

	if d := c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s1", origin: launchOriginAutomatic, seam: "seam2"}); !d.admitted {
		t.Fatalf("first admit = %+v, want admitted", d)
	}
	if d := c.admit(context.Background(), admissionRequest{taskID: "t2", sessionID: "s2", origin: launchOriginAutomatic, seam: "seam2"}); d.admitted {
		t.Fatalf("second admit = %+v, want refused", d)
	}
}

// A failed population query fails CLOSED for automatic launches and open for manual
// ones, and neither surface may report a number it does not have.
func TestUnknownPopulationFailsClosedForAutomaticAndOpenForManual(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.fail(errors.New("database is locked"))
	c := newTestController(t, 4, lister)

	auto := c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s1", origin: launchOriginAutomatic, seam: "seam2"})
	if auto.admitted {
		t.Fatalf("automatic admit = %+v, want refused when the population is unknown", auto)
	}
	if auto.reasonCode != ceilingReasonUnknownPopulation {
		t.Fatalf("automatic reasonCode = %q, want %q", auto.reasonCode, ceilingReasonUnknownPopulation)
	}
	if auto.populationKnown {
		t.Fatal("automatic decision reported the population as known")
	}

	manual := c.admit(context.Background(), admissionRequest{taskID: "t2", sessionID: "s2", origin: launchOriginManual, seam: "seam2"})
	if !manual.admitted || !manual.manualOverride {
		t.Fatalf("manual admit = %+v, want admitted as an override", manual)
	}
	if manual.populationKnown {
		t.Fatal("manual decision reported the population as known")
	}
	if manual.reasonCode != ceilingReasonUnknownPopulation {
		t.Fatalf("manual reasonCode = %q, want %q", manual.reasonCode, ceilingReasonUnknownPopulation)
	}
}

// The failure concerns the counted rows only: reservations already held stay held.
func TestUnknownPopulationDoesNotReleaseHeldReservations(t *testing.T) {
	lister := &fakeAdmittedLister{}
	c := newTestController(t, 4, lister)
	c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s1", origin: launchOriginAutomatic, seam: "seam2"})

	lister.fail(errors.New("database is locked"))
	c.admit(context.Background(), admissionRequest{taskID: "t2", sessionID: "s2", origin: launchOriginAutomatic, seam: "seam2"})

	lister.fail(nil)
	if got := mustPopulation(t, c); got != 1 {
		t.Fatalf("population after a failed query = %d, want 1 (the held reservation survived)", got)
	}
}

// An unlabelled launch is automatic, so it is refused and deferred rather than
// silently granted an override.
func TestMissingOriginIsTreatedAsAutomatic(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1", "s2")
	c := newTestController(t, 2, lister)

	d := c.admit(context.Background(), admissionRequest{taskID: "t1", sessionID: "s9", seam: "seam4"})
	if d.admitted {
		t.Fatalf("admit with no origin = %+v, want refused as automatic", d)
	}
}

// Two launches racing for one free slot produce exactly one admission.
func TestConcurrentAdmissionsForOneFreeSlotAdmitExactlyOne(t *testing.T) {
	lister := &fakeAdmittedLister{}
	lister.set("s1")
	c := newTestController(t, 2, lister)

	const racers = 32
	var wg sync.WaitGroup
	results := make([]admissionDecision, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = c.admit(context.Background(), admissionRequest{
				taskID:    fmt.Sprintf("t%d", i),
				sessionID: fmt.Sprintf("race-%d", i),
				origin:    launchOriginAutomatic,
				seam:      "seam2",
			})
		}(i)
	}
	close(start)
	wg.Wait()

	admitted := 0
	for _, d := range results {
		if d.admitted {
			admitted++
		}
	}
	if admitted != 1 {
		t.Fatalf("%d of %d racers admitted, want exactly 1", admitted, racers)
	}
	if got := mustPopulation(t, c); got != 2 {
		t.Fatalf("population = %d, want 2 (the ceiling)", got)
	}
}
