package auth

import (
	"testing"
	"time"
)

func TestLoginLimiterWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	limiter := newLoginLimiter(5*time.Minute, 3)
	limiter.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !limiter.Allow("k") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if limiter.Allow("k") {
		t.Fatal("4th attempt within the window must be blocked")
	}
	// Other keys are unaffected.
	if !limiter.Allow("other") {
		t.Fatal("independent key must be allowed")
	}
	// Window expiry resets the count.
	now = now.Add(5 * time.Minute)
	if !limiter.Allow("k") {
		t.Fatal("attempt after window expiry must be allowed")
	}
}

func TestLoginLimiterReset(t *testing.T) {
	limiter := newLoginLimiter(5*time.Minute, 1)
	if !limiter.Allow("k") {
		t.Fatal("first attempt allowed")
	}
	if limiter.Allow("k") {
		t.Fatal("second attempt blocked")
	}
	limiter.Reset("k")
	if !limiter.Allow("k") {
		t.Fatal("attempt after reset must be allowed")
	}
}
