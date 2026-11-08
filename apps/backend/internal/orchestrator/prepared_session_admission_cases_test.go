package orchestrator

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// With authentication on, an internal start of an unattributed prepared
// session has nobody to launch as. It fails closed with an actionable error
// instead of reaching the runtime with an unscoped context.
func TestPreparedSessionAdmission_UnattributedSessionFailsClosed(t *testing.T) {
	sc := buildPreparedAdmissionScenario(t, preparedAdmissionOptions{})
	session := sc.assertFailedClosed(t, sc.start(sc.ctx, t))
	if !strings.Contains(session.ErrorMessage, "executor actor") {
		t.Errorf("session error = %q, want the missing executor actor named", session.ErrorMessage)
	}
}

// A durable actor whose account was deleted or disabled no longer resolves.
// The start fails closed rather than continuing without an identity.
func TestPreparedSessionAdmission_UnavailableDurableActorFailsClosed(t *testing.T) {
	sc := buildPreparedAdmissionScenario(t, preparedAdmissionOptions{sessionActor: preparedDeleted})
	session := sc.assertFailedClosed(t, sc.start(sc.ctx, t))
	if !strings.Contains(session.ErrorMessage, "executor actor") {
		t.Errorf("session error = %q, want the missing executor actor named", session.ErrorMessage)
	}
}

func TestPreparedSessionAdmission_CollaboratorSessionStartsAsCollaborator(t *testing.T) {
	sc := buildPreparedAdmissionScenario(t, preparedAdmissionOptions{sessionActor: preparedCollaborator})
	calls := sc.assertStartedAs(t, sc.start(sc.ctx, t), preparedCollaborator)
	sc.assertReveals(t, calls, preparedCollabSecret, preparedOwnerSecret)
}

func TestPreparedSessionAdmission_OwnerSessionStartsAsOwner(t *testing.T) {
	sc := buildPreparedAdmissionScenario(t, preparedAdmissionOptions{sessionActor: preparedOwner})
	calls := sc.assertStartedAs(t, sc.start(sc.ctx, t), preparedOwner)
	sc.assertReveals(t, calls, preparedOwnerSecret, preparedCollabSecret)
}

// An authorized caller takes precedence over the durable actor.
func TestPreparedSessionAdmission_AuthorizedCallerTakesPrecedence(t *testing.T) {
	sc := buildPreparedAdmissionScenario(t, preparedAdmissionOptions{sessionActor: preparedCollaborator})
	calls := sc.assertStartedAs(t, sc.start(preparedCallerContext(preparedOwner), t), preparedOwner)
	sc.assertReveals(t, calls, preparedOwnerSecret, preparedCollabSecret)
}

// A caller from outside the workspace is refused and the start never falls
// back to the session's privileged durable actor.
func TestPreparedSessionAdmission_UnauthorizedCallerDoesNotFallBack(t *testing.T) {
	sc := buildPreparedAdmissionScenario(t, preparedAdmissionOptions{sessionActor: preparedOwner})
	sc.assertFailedClosed(t, sc.start(preparedCallerContext(preparedStranger), t))
	for _, a := range sc.admissionCalls() {
		if a.err == nil {
			t.Errorf("admission accepted the foreign caller as %s", a.identity)
		}
	}
}

// With authentication off every actor resolves to the synthetic identity, so
// single-user installs start prepared sessions exactly as before.
func TestPreparedSessionAdmission_AuthDisabledStartsWithSyntheticIdentity(t *testing.T) {
	sc := buildPreparedAdmissionScenario(t, preparedAdmissionOptions{authDisabled: true})
	sc.assertStartedAs(t, sc.start(sc.ctx, t), "<synthetic>")
}

// The executor profile is private to the workspace owner and the session was
// prepared by the collaborator. Admission refuses the collaborator and the
// start is not retried under anyone else's authority.
func TestPreparedSessionAdmission_PrivateProfileIsNotBorrowed(t *testing.T) {
	sc := buildPreparedAdmissionScenario(t, preparedAdmissionOptions{sessionActor: preparedCollaborator, profileOwner: preparedOwner})
	sc.assertFailedClosed(t, sc.start(sc.ctx, t))
	admissions := sc.admissionCalls()
	if len(admissions) == 0 {
		t.Fatal("executor admission never ran for the prepared session")
	}
	for _, a := range admissions {
		if a.err == nil {
			t.Errorf("admission accepted the owner's private profile for %s", a.identity)
		}
	}
}

// A cancelled starter launches nothing under any identity.
func TestPreparedSessionAdmission_CancelledStartLaunchesNothing(t *testing.T) {
	sc := buildPreparedAdmissionScenario(t, preparedAdmissionOptions{})
	ctx, cancel := context.WithCancel(sc.ctx)
	cancel()
	_ = sc.start(ctx, t)
	for _, call := range sc.runtimeCalls() {
		t.Errorf("cancelled start reached the runtime: %s as %s", call.name, preparedIdentity(call.ctx))
	}
}

// Concurrent and duplicate starts share one admission-gated launch.
func TestPreparedSessionAdmission_ConcurrentStartsLaunchOnce(t *testing.T) {
	sc := buildPreparedAdmissionScenario(t, preparedAdmissionOptions{sessionActor: preparedCollaborator})
	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = sc.start(sc.ctx, t)
		}(i)
	}
	wg.Wait()
	time.Sleep(100 * time.Millisecond)
	starts := 0
	for _, call := range sc.runtimeCalls() {
		if got := preparedIdentity(call.ctx); got != preparedCollaborator {
			t.Errorf("%s carried identity %s, want %s", call.name, got, preparedCollaborator)
		}
		if call.name == "StartAgentProcess" {
			starts++
		}
	}
	if starts != 1 {
		t.Errorf("agent process starts = %d, want 1 (errors: %v)", starts, errs)
	}
}

// Concurrent unattributed starts of the same CREATED session are each refused
// and none reaches the runtime. A later start of the resulting FAILED session
// takes the cold ResumeSession branch, which is outside this fixture.
func TestPreparedSessionAdmission_ConcurrentUnattributedStartsStayClosed(t *testing.T) {
	sc := buildPreparedAdmissionScenario(t, preparedAdmissionOptions{})
	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = sc.start(sc.ctx, t)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err == nil {
			t.Errorf("unattributed start %d succeeded, want it refused", i)
		}
	}
	if calls := sc.runtimeCalls(); len(calls) != 0 {
		t.Errorf("unattributed start reached the runtime %d times", len(calls))
	}
}
