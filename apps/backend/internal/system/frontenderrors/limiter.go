package frontenderrors

import (
	"math"
	"sync"
	"time"
)

const (
	identityRatePerMinute  = 60
	identityBurst          = 20
	globalRatePerMinute    = 300
	globalBurst            = 100
	identityBytesPerMinute = 64 * 1024
	identityBytesBurst     = 64 * 1024
	globalBytesPerMinute   = 256 * 1024
	globalBytesBurst       = 256 * 1024
	identityBucketTTL      = 10 * time.Minute
	limiterCleanupEvery    = time.Minute
)

type tokenBucket struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

type identityBuckets struct {
	requests tokenBucket
	bytes    tokenBucket
	lastSeen time.Time
}

type limiter struct {
	mu             sync.Mutex
	now            func() time.Time
	globalRequests tokenBucket
	globalBytes    tokenBucket
	identities     map[string]*identityBuckets
	lastCleanup    time.Time
}

func newLimiter(now func() time.Time) *limiter {
	if now == nil {
		now = time.Now
	}
	current := now()
	return &limiter{
		now:            now,
		globalRequests: newTokenBucket(globalBurst, current),
		globalBytes:    newTokenBucket(globalBytesBurst, current),
		identities:     make(map[string]*identityBuckets),
		lastCleanup:    current,
	}
}

func (l *limiter) Allow(identity string) (bool, time.Duration) {
	return l.allow(identity, 1, false)
}

func (l *limiter) AllowBytes(identity string, size int) (bool, time.Duration) {
	return l.allow(identity, max(1, size), true)
}

func (l *limiter) allow(identity string, cost int, bytes bool) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.cleanup(now)
	buckets := l.identities[identity]
	if buckets == nil {
		buckets = &identityBuckets{
			requests: newTokenBucket(identityBurst, now),
			bytes:    newTokenBucket(identityBytesBurst, now),
			lastSeen: now,
		}
		l.identities[identity] = buckets
	}
	buckets.lastSeen = now
	if bytes {
		return consumeBuckets(&buckets.bytes, &l.globalBytes, cost,
			identityBytesPerMinute, identityBytesBurst,
			globalBytesPerMinute, globalBytesBurst, now)
	}
	return consumeBuckets(&buckets.requests, &l.globalRequests, cost,
		identityRatePerMinute, identityBurst, globalRatePerMinute, globalBurst, now)
}

func consumeBuckets(identity, global *tokenBucket, cost, identityRate, identityBurst,
	globalRate, globalBurst int, now time.Time) (bool, time.Duration) {
	refill(identity, identityRate, identityBurst, now)
	refill(global, globalRate, globalBurst, now)
	identityRetry := retryAfter(identity, identityRate, cost)
	globalRetry := retryAfter(global, globalRate, cost)
	if identityRetry > 0 || globalRetry > 0 {
		return false, maxDuration(identityRetry, globalRetry)
	}
	identity.tokens -= float64(cost)
	global.tokens -= float64(cost)
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

func retryAfter(bucket *tokenBucket, ratePerMinute, cost int) time.Duration {
	if bucket.tokens >= float64(cost) {
		return 0
	}
	seconds := math.Ceil((float64(cost) - bucket.tokens) / (float64(ratePerMinute) / 60))
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
