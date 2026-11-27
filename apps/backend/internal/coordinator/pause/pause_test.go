package pause

import (
	"context"
	"errors"
	"testing"
)

var errRead = errors.New("read failed")

func newTestGate(read ReadFunc, effective bool, known *KnownSet) Gate {
	return NewGate(read, func() bool { return effective }, known, nil)
}

func TestGateReadsStoredState(t *testing.T) {
	for _, want := range []bool{true, false} {
		g := newTestGate(func(context.Context, string) (bool, error) { return want, nil }, false, NewKnownSet())
		got, err := g.Active(context.Background(), "c1")
		if err != nil || got != want {
			t.Fatalf("Active = (%v, %v), want (%v, nil)", got, err, want)
		}
	}
}

func TestGateReadErrorFailsClosedWithFlagEffective(t *testing.T) {
	g := newTestGate(func(context.Context, string) (bool, error) { return false, errRead }, true, NewKnownSet())
	got, err := g.Active(context.Background(), "c1")
	if !got || !errors.Is(err, errRead) {
		t.Fatalf("Active = (%v, %v), want (true, errRead)", got, err)
	}
}

func TestGateReadErrorFlagOffCarveOut(t *testing.T) {
	known := NewKnownSet()
	known.Set("paused", true)
	g := newTestGate(func(context.Context, string) (bool, error) { return false, errRead }, false, known)

	if got, err := g.Active(context.Background(), "paused"); !got || err == nil {
		t.Fatalf("known-paused Active = (%v, %v), want (true, err)", got, err)
	}
	if got, err := g.Active(context.Background(), "other"); got || !errors.Is(err, errRead) {
		t.Fatalf("other Active = (%v, %v), want (false, errRead)", got, err)
	}
}

func TestGateEmptyIDIsReadError(t *testing.T) {
	called := false
	g := newTestGate(func(context.Context, string) (bool, error) { called = true; return false, nil }, true, NewKnownSet())
	got, err := g.Active(context.Background(), "")
	if !got || !errors.Is(err, ErrEmptyID) || called {
		t.Fatalf("Active = (%v, %v), read called = %v", got, err, called)
	}
}

func TestKnownSetSetAndReplace(t *testing.T) {
	k := NewKnownSet()
	k.Set("a", true)
	k.Set("b", true)
	k.Set("b", false)
	if !k.Has("a") || k.Has("b") {
		t.Fatalf("Set: a=%v b=%v", k.Has("a"), k.Has("b"))
	}
	k.Replace([]string{"c"})
	if k.Has("a") || !k.Has("c") {
		t.Fatalf("Replace: a=%v c=%v", k.Has("a"), k.Has("c"))
	}
}
