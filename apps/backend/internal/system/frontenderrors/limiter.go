package frontenderrors

import (
	"math"
	"sync"
	"time"
)

const (
	identityRatePerMinute = 60
	identityBurst         = 20
	globalRatePerMinute   = 300
	globalBurst           = 100
	identityBucketTTL     = 10 * time.Minute
	limiterCleanupEvery   = time.Minute
)

type tokenBucket struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

type limiter struct {
	mu          sync.Mutex
	now         func() time.Time
	global      tokenBucket
	identities  map[string]*tokenBucket
	lastCleanup time.Time
}

func newLimiter(now func() time.Time) *limiter {
	if now == nil {
		now = time.Now
	}
	current := now()
	return &limiter{
		now: now, global: newTokenBucket(globalBurst, current),
		identities: make(map[string]*tokenBucket), lastCleanup: current,
	}
}

func (l *limiter) Allow(identity string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.cleanup(now)
	identityBucket := l.identities[identity]
	if identityBucket == nil {
		bucket := newTokenBucket(identityBurst, now)
		identityBucket = &bucket
		l.identities[identity] = identityBucket
	}
	refill(identityBucket, identityRatePerMinute, identityBurst, now)
	refill(&l.global, globalRatePerMinute, globalBurst, now)
	identityBucket.lastSeen = now
	identityRetry := retryAfter(identityBucket, identityRatePerMinute)
	globalRetry := retryAfter(&l.global, globalRatePerMinute)
	if identityRetry > 0 || globalRetry > 0 {
		return false, maxDuration(identityRetry, globalRetry)
	}
	identityBucket.tokens--
	l.global.tokens--
	return true, 0
}

func (l *limiter) cleanup(now time.Time) {
	if now.Sub(l.lastCleanup) < limiterCleanupEvery {
		return
	}
	for identity, bucket := range l.identities {
		if now.Sub(bucket.lastSeen) > identityBucketTTL {
			delete(l.identities, identity)
		}
	}
	l.lastCleanup = now
}

func newTokenBucket(burst int, now time.Time) tokenBucket {
	return tokenBucket{tokens: float64(burst), last: now, lastSeen: now}
}

func refill(bucket *tokenBucket, ratePerMinute, burst int, now time.Time) {
	elapsed := now.Sub(bucket.last).Minutes()
	if elapsed > 0 {
		bucket.tokens = math.Min(float64(burst), bucket.tokens+elapsed*float64(ratePerMinute))
		bucket.last = now
	}
}

func retryAfter(bucket *tokenBucket, ratePerMinute int) time.Duration {
	if bucket.tokens >= 1 {
		return 0
	}
	seconds := math.Ceil((1 - bucket.tokens) / (float64(ratePerMinute) / 60))
	if seconds < 1 {
		seconds = 1
	}
	return time.Duration(seconds) * time.Second
}

func maxDuration(left, right time.Duration) time.Duration {
	if left > right {
		return left
	}
	return right
}
