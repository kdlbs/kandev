package backendapp

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const e2eOnlyMockMode = "only"
const e2eEnabledValue = "true"

type e2eCapacityRetryClock struct {
	mu      sync.Mutex
	frozen  bool
	now     time.Time
	waiters map[chan struct{}]time.Time
}

func newE2ECapacityRetryClock() *e2eCapacityRetryClock {
	if !capacityRetryClockMockEnabled() {
		return nil
	}
	return &e2eCapacityRetryClock{waiters: make(map[chan struct{}]time.Time)}
}

func (c *e2eCapacityRetryClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frozen {
		return c.now
	}
	return time.Now()
}

func (c *e2eCapacityRetryClock) freeze() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frozen || len(c.waiters) != 0 {
		return false
	}
	c.now, c.frozen = time.Now(), true
	return true
}

func (c *e2eCapacityRetryClock) advance(delay time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.frozen {
		return false
	}
	c.now = c.now.Add(delay)
	for ready, due := range c.waiters {
		if !c.now.Before(due) {
			delete(c.waiters, ready)
			close(ready)
		}
	}
	return true
}

func (c *e2eCapacityRetryClock) WaitUntil(ctx context.Context, due time.Time) bool {
	if ctx.Err() != nil {
		return false
	}
	c.mu.Lock()
	if !c.frozen {
		c.mu.Unlock()
		timer := time.NewTimer(time.Until(due))
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return ctx.Err() == nil
		}
	}
	if !c.now.Before(due) {
		c.mu.Unlock()
		return ctx.Err() == nil
	}
	ready := make(chan struct{})
	c.waiters[ready] = due
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.waiters, ready); c.mu.Unlock() }()
	select {
	case <-ctx.Done():
		return false
	case <-ready:
		return ctx.Err() == nil
	}
}

func registerE2ECapacityRetryClockRoutes(router *gin.Engine, clock *e2eCapacityRetryClock) {
	if clock == nil || !capacityRetryClockMockEnabled() {
		return
	}
	api := router.Group("/api/v1/e2e/capacity-retry-clock")
	api.POST("/freeze", func(c *gin.Context) {
		if !clock.freeze() {
			c.Status(http.StatusConflict)
			return
		}
		c.Status(http.StatusNoContent)
	})
	api.POST("/advance", func(c *gin.Context) {
		var request struct {
			Seconds int64 `json:"seconds"`
		}
		if err := c.ShouldBindJSON(&request); err != nil || request.Seconds < 0 || request.Seconds > 1800 {
			c.Status(http.StatusBadRequest)
			return
		}
		if !clock.advance(time.Duration(request.Seconds) * time.Second) {
			c.Status(http.StatusConflict)
			return
		}
		c.Status(http.StatusNoContent)
	})
}

func capacityRetryClockMockEnabled() bool {
	mode := os.Getenv("KANDEV_MOCK_AGENT")
	return os.Getenv("KANDEV_E2E_MOCK") == e2eEnabledValue && (mode == e2eEnabledValue || mode == e2eOnlyMockMode)
}
