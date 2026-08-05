package websocket

import "testing"

func TestLSPCapacityLimiterRejectsAboveMaxAndReleases(t *testing.T) {
	limiter := newLSPCapacityLimiter(1)
	if !limiter.TryAcquire() {
		t.Fatal("first acquire should succeed")
	}
	if limiter.TryAcquire() {
		t.Fatal("second acquire should fail at capacity")
	}
	limiter.Release()
	if !limiter.TryAcquire() {
		t.Fatal("acquire after release should succeed")
	}
}

func TestLSPCapacityLimiterFallsBackToDefaultForInvalidEnv(t *testing.T) {
	t.Setenv("KANDEV_LSP_MAX_CONNECTIONS", "not-a-number")
	limiter := newLSPCapacityLimiterFromEnv()
	if limiter.max != defaultLSPMaxConnections {
		t.Fatalf("max = %d, want %d", limiter.max, defaultLSPMaxConnections)
	}
}
