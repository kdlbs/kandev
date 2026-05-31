package health

import (
	"context"
	"testing"
	"time"
)

// countingChecker tracks how many times Check is called.
type countingChecker struct {
	calls  int
	issues []Issue
}

func (c *countingChecker) Check(_ context.Context) []Issue {
	c.calls++
	return c.issues
}

func (c *countingChecker) Name() string     { return "counting" }
func (c *countingChecker) Category() string { return "test" }

func TestCachedChecker_ReturnsInnerResult(t *testing.T) {
	inner := &countingChecker{issues: []Issue{{ID: "test_issue", Severity: SeverityWarning}}}
	cc := NewCachedChecker(inner, time.Minute)

	got := cc.Check(context.Background())
	if len(got) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(got))
	}
	if got[0].ID != "test_issue" {
		t.Errorf("ID = %q, want %q", got[0].ID, "test_issue")
	}
}

func TestCachedChecker_CacheHitOnSecondCall(t *testing.T) {
	inner := &countingChecker{issues: []Issue{{ID: "test_issue", Severity: SeverityWarning}}}
	cc := NewCachedChecker(inner, time.Minute)

	cc.Check(context.Background())
	cc.Check(context.Background())

	if inner.calls != 1 {
		t.Errorf("inner called %d times, want 1 (second call should hit cache)", inner.calls)
	}
}

func TestCachedChecker_CacheExpiresAfterTTL(t *testing.T) {
	inner := &countingChecker{issues: []Issue{{ID: "test_issue", Severity: SeverityWarning}}}
	cc := NewCachedChecker(inner, time.Millisecond)

	cc.Check(context.Background())
	time.Sleep(5 * time.Millisecond)
	cc.Check(context.Background())

	if inner.calls != 2 {
		t.Errorf("inner called %d times, want 2 (second call after TTL expiry)", inner.calls)
	}
}

func TestCachedChecker_NilResultIsCached(t *testing.T) {
	inner := &countingChecker{issues: nil}
	cc := NewCachedChecker(inner, time.Minute)

	got := cc.Check(context.Background())
	if got != nil {
		t.Errorf("expected nil result, got %v", got)
	}

	got2 := cc.Check(context.Background())
	if got2 != nil {
		t.Errorf("expected nil result on second call, got %v", got2)
	}

	if inner.calls != 1 {
		t.Errorf("inner called %d times, want 1 (nil result should be cached)", inner.calls)
	}
}

func TestCachedChecker_DelegatesNameAndCategory(t *testing.T) {
	inner := &countingChecker{}
	cc := NewCachedChecker(inner, time.Minute)
	if cc.Name() != "counting" {
		t.Errorf("Name() = %q, want %q", cc.Name(), "counting")
	}
	if cc.Category() != "test" {
		t.Errorf("Category() = %q, want %q", cc.Category(), "test")
	}
}
