package frontenderrors

import (
	"fmt"
	"testing"
	"time"
)

func TestLimiterEnforcesIdentityAndGlobalBuckets(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	limiter := newLimiter(func() time.Time { return now })
	for index := 0; index < identityBurst; index++ {
		if allowed, _ := limiter.Allow("one-user"); !allowed {
			t.Fatalf("identity request %d rejected", index)
		}
	}
	if allowed, retry := limiter.Allow("one-user"); allowed || retry < time.Second {
		t.Fatalf("identity overflow allowed=%v retry=%s", allowed, retry)
	}

	limiter = newLimiter(func() time.Time { return now })
	for index := 0; index < globalBurst; index++ {
		if allowed, _ := limiter.Allow(fmt.Sprintf("user-%d", index)); !allowed {
			t.Fatalf("global request %d rejected", index)
		}
	}
	if allowed, retry := limiter.Allow("overflow-user"); allowed || retry <= 0 {
		t.Fatalf("global overflow allowed=%v retry=%s", allowed, retry)
	}
}

func TestLimiterRefillsAndRemovesStaleIdentities(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	limiter := newLimiter(func() time.Time { return now })
	for range identityBurst {
		limiter.Allow("stale-user")
	}
	now = now.Add(identityBucketTTL + time.Minute)
	if allowed, _ := limiter.Allow("new-user"); !allowed {
		t.Fatal("request after refill was rejected")
	}
	if _, exists := limiter.identities["stale-user"]; exists {
		t.Fatal("stale identity bucket was not removed")
	}
}
