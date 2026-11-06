package changes

import (
	"context"
	"testing"
	"time"
)

func TestExportLimiterBoundsInstallationConcurrency(t *testing.T) {
	limiter := NewExportLimiter()
	first := limiter.Acquire(context.Background())
	second := limiter.Acquire(context.Background())
	if !first || !second {
		t.Fatal("first two slots should be admitted")
	}
	blocked, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if limiter.Acquire(blocked) {
		t.Fatal("third concurrent export must be refused")
	}
	limiter.Release()
	if !limiter.Acquire(context.Background()) {
		t.Fatal("released capacity should admit the next export")
	}
	limiter.Release()
	limiter.Release()
}
