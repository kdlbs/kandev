package updates

import (
	"testing"
	"time"
)

func TestLimiter_FirstCallAllowed(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewLimiter(30*time.Second, WithClock(func() time.Time { return now }))
	ok, retry := l.Allow()
	if !ok {
		t.Fatalf("expected first call to be allowed")
	}
	if retry != 0 {
		t.Fatalf("expected retry=0 when allowed, got %v", retry)
	}
}

func TestLimiter_SecondCallWithinWindowDenied(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewLimiter(30*time.Second, WithClock(func() time.Time { return now }))
	if ok, _ := l.Allow(); !ok {
		t.Fatalf("seed call should be allowed")
	}
	// advance 5s
	now = now.Add(5 * time.Second)
	ok, retry := l.Allow()
	if ok {
		t.Fatalf("expected second call within window to be denied")
	}
	if retry <= 0 || retry > 30*time.Second {
		t.Fatalf("expected positive retry <=30s, got %v", retry)
	}
	wantRetry := 25 * time.Second
	if retry != wantRetry {
		t.Fatalf("expected retry %v, got %v", wantRetry, retry)
	}
}

func TestLimiter_AfterWindowAllowedAgain(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	l := NewLimiter(30*time.Second, WithClock(func() time.Time { return now }))
	if ok, _ := l.Allow(); !ok {
		t.Fatalf("seed call should be allowed")
	}
	now = now.Add(31 * time.Second)
	ok, retry := l.Allow()
	if !ok {
		t.Fatalf("expected call after window to be allowed")
	}
	if retry != 0 {
		t.Fatalf("expected retry=0, got %v", retry)
	}
}
