package coordinator

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type resolverCall struct{ taskID, sessionID, pendingID, turnID string }

type fakePermissionResolver struct {
	mu    sync.Mutex
	calls []resolverCall
	err   error
}

func (f *fakePermissionResolver) ResolveUnattendedPermission(_ context.Context, taskID, sessionID, pendingID, turnID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, resolverCall{taskID, sessionID, pendingID, turnID})
	return f.err
}

func newUnattendedFixture(t *testing.T) (*Service, *Store, *Coordinator, *fakePermissionResolver) {
	t.Helper()
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	svc := NewService(store, NewValidator(nil, nil), &fakeWorkspaceAuthorizer{}, newTestLogger(t))
	resolver := &fakePermissionResolver{}
	svc.SetUnattendedPermissionResolver(resolver)
	return svc, store, c, resolver
}

func TestHandleUnattendedPermission_DeniesAndCountsOnce(t *testing.T) {
	svc, store, c, resolver := newUnattendedFixture(t)
	insertBoundTurn(t, store, c, "turn-1", "conv", "sess", "st-1", nil)

	svc.HandleUnattendedPermission(context.Background(), "conv", "sess", "pending-1", "st-1")
	svc.HandleUnattendedPermission(context.Background(), "conv", "sess", "pending-1", "st-1")

	want := resolverCall{"conv", "sess", "pending-1", "turn-1"}
	if len(resolver.calls) != 2 || resolver.calls[0] != want || resolver.calls[1] != want {
		t.Fatalf("resolver calls = %+v, want the turn row id on both (a redelivery still resolves)", resolver.calls)
	}
	if got := deniedCount(t, store, "turn-1"); got != 1 {
		t.Fatalf("denied_permissions = %d, want 1", got)
	}
}

func TestHandleUnattendedPermission_UnboundWindowIsDeniedAndCounted(t *testing.T) {
	svc, store, c, resolver := newUnattendedFixture(t)
	insertBoundTurn(t, store, c, "turn-1", "conv", "sess", nil, nil)

	svc.HandleUnattendedPermission(context.Background(), "conv", "sess", "pending-1", "st-1")

	if len(resolver.calls) != 1 || resolver.calls[0].turnID != "turn-1" {
		t.Fatalf("resolver calls = %+v, want one denial while session_turn_id is unbound", resolver.calls)
	}
	if got := deniedCount(t, store, "turn-1"); got != 1 {
		t.Fatalf("denied_permissions = %d, want 1", got)
	}
}

func TestHandleUnattendedPermission_NoMatchWaitsForAPerson(t *testing.T) {
	svc, store, c, resolver := newUnattendedFixture(t)
	insertBoundTurn(t, store, c, "turn-1", "conv", "sess", "st-1", nil)

	svc.HandleUnattendedPermission(context.Background(), "conv", "sess", "pending-1", "st-later")
	svc.HandleUnattendedPermission(context.Background(), "other-conv", "sess", "pending-1", "st-1")

	if len(resolver.calls) != 0 || denialRows(t, store, "turn-1") != 0 {
		t.Fatalf("calls=%+v rows=%d, want nothing recorded or resolved", resolver.calls, denialRows(t, store, "turn-1"))
	}
}

func TestHandleUnattendedPermission_NilResolverDoesNothing(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	svc := NewService(store, NewValidator(nil, nil), &fakeWorkspaceAuthorizer{}, newTestLogger(t))
	insertBoundTurn(t, store, c, "turn-1", "conv", "sess", "st-1", nil)

	svc.HandleUnattendedPermission(context.Background(), "conv", "sess", "pending-1", "st-1")

	if denialRows(t, store, "turn-1") != 0 {
		t.Fatal("without a resolver nothing may be recorded")
	}
}

func TestHandleUnattendedPermission_FailedResolutionKeepsDenialForRetry(t *testing.T) {
	svc, store, c, resolver := newUnattendedFixture(t)
	insertBoundTurn(t, store, c, "turn-1", "conv", "sess", "st-1", nil)
	resolver.err = errors.New("delivery failed")

	svc.HandleUnattendedPermission(context.Background(), "conv", "sess", "pending-1", "st-1")
	if denialRows(t, store, "turn-1") != 1 || deniedCount(t, store, "turn-1") != 1 {
		t.Fatal("a failed resolution must leave the recorded denial in place")
	}

	resolver.err = nil
	resolver.calls = nil
	if err := svc.ReresolveRecordedDenials(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != (resolverCall{"conv", "sess", "pending-1", "turn-1"}) {
		t.Fatalf("re-resolve calls = %+v", resolver.calls)
	}
	if got := deniedCount(t, store, "turn-1"); got != 1 {
		t.Fatalf("re-resolution counted again: denied_permissions = %d", got)
	}
}

func TestReresolveRecordedDenials_OnlyThisCoordinatorsOpenTurns(t *testing.T) {
	svc, store, c, resolver := newUnattendedFixture(t)
	other := newTestCoordinator(t, store, "ws-1")
	insertBoundTurn(t, store, c, "open", "conv", "sess", "st-1", nil)
	insertBoundTurn(t, store, c, "settled", "conv", "sess-old", "st-0", "completed")
	insertBoundTurn(t, store, other, "other-open", "conv-o", "sess-o", "st-o", nil)
	insertDenial(t, store, "open", "p-1")
	insertDenial(t, store, "settled", "p-2")
	insertDenial(t, store, "other-open", "p-3")

	if err := svc.ReresolveRecordedDenials(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != (resolverCall{"conv", "sess", "p-1", "open"}) {
		t.Fatalf("calls = %+v, want only this coordinator's open turn", resolver.calls)
	}
	if deniedCount(t, store, "open") != 0 {
		t.Fatal("re-resolution must not count")
	}
}

func TestReresolveRecordedDenials_EmptyAndNilResolverAreSuccess(t *testing.T) {
	svc, _, c, resolver := newUnattendedFixture(t)
	if err := svc.ReresolveRecordedDenials(context.Background(), c.ID); err != nil || len(resolver.calls) != 0 {
		t.Fatalf("empty selection: err=%v calls=%+v", err, resolver.calls)
	}
	svc.SetUnattendedPermissionResolver(nil)
	if err := svc.ReresolveRecordedDenials(context.Background(), c.ID); err != nil {
		t.Fatalf("nil resolver: %v", err)
	}
}

func TestReresolveRecordedDenials_ReturnsFirstFailureAfterTryingAll(t *testing.T) {
	svc, store, c, resolver := newUnattendedFixture(t)
	insertBoundTurn(t, store, c, "open", "conv", "sess", "st-1", nil)
	insertDenial(t, store, "open", "p-1")
	insertDenial(t, store, "open", "p-2")
	resolver.err = errors.New("boom")

	if err := svc.ReresolveRecordedDenials(context.Background(), c.ID); err == nil {
		t.Fatal("a failed re-resolution must be reported")
	}
	if len(resolver.calls) != 2 {
		t.Fatalf("calls = %d, want both denials tried", len(resolver.calls))
	}
}
