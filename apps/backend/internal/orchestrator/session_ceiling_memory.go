package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/system/hostmemory"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// ceilingReasonMemoryPressure refuses a launch that would add an agent process
// while the host is low on available memory or above its commit ceiling. It is
// distinct from the session-count reasons so an operator can tell a host
// resource limit from a count limit.
const ceilingReasonMemoryPressure = "memory_pressure"

// memoryPressureReader is the narrow host-memory seam the admission controller
// consults. It is satisfied by hostmemory.Reader.
type memoryPressureReader interface {
	Read(ctx context.Context) (hostmemory.Reading, error)
}

// memoryGateExemption reports whether a task's launches are exempt from the
// host-memory condition. Office coordinator conversations are exempt so the
// agent that manages the other sessions keeps running.
type memoryGateExemption func(ctx context.Context, taskID string) bool

// errMemoryReadingIncomplete marks a usable reader that reported an incomplete
// reading (an unsupported platform, for example). It is not a host failure.
var errMemoryReadingIncomplete = errors.New("host memory reading is incomplete")

// memorySampleTTL bounds how long one complete host reading is reused. Launches
// arrive in bursts, and the admission decisions inside a burst are separated by
// milliseconds, so one sample per second keeps the lock short without letting
// pressure lag behind.
const memorySampleTTL = time.Second

// configureMemoryGate enables the host-memory condition on the controller. A
// disabled gate leaves the reader nil, so the host is never read.
func (c *sessionCeilingController) configureMemoryGate(
	gate hostmemory.Gate, reader memoryPressureReader, exempt memoryGateExemption,
) {
	if c == nil || !gate.Enabled || reader == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memoryGate = gate
	c.memoryReader = reader
	c.memoryExempt = exempt
}

// memoryPressuredLocked reports whether the host is under memory pressure for
// this launch. Manual launches are never judged, and the host is not read for
// them, so a user-initiated start, resume, or prompt keeps its existing
// admission and its input. A disabled gate, an exempt task, or an unreadable or
// incomplete reading report "not pressured" (fail open) so an unmeasurable host
// never stalls launches.
func (c *sessionCeilingController) memoryPressuredLocked(ctx context.Context, origin launchOrigin, req admissionRequest) bool {
	if c == nil || !c.memoryGate.Enabled || c.memoryReader == nil {
		return false
	}
	if origin == launchOriginManual {
		return false
	}
	reading, ok := c.memoryReadingLocked(ctx)
	if !ok {
		return false
	}
	if pressured, _ := c.memoryGate.Pressured(reading); !pressured {
		return false
	}
	// The exemption reads the task, so it is consulted only when the host is
	// actually pressured.
	if c.memoryExempt != nil && c.memoryExempt(ctx, req.taskID) {
		return false
	}
	return true
}

// memoryReadingLocked returns a complete host reading, reusing one sample inside
// memorySampleTTL. A failed or incomplete reading is never cached, so the next
// launch probes again and a failure streak keeps its single warning.
func (c *sessionCeilingController) memoryReadingLocked(ctx context.Context) (hostmemory.Reading, bool) {
	now := c.now()
	if c.memorySamplePresent && now.Sub(c.memorySampleAt) < memorySampleTTL {
		return c.memorySample, true
	}
	reading, err := c.memoryReader.Read(ctx)
	if err != nil {
		c.memorySamplePresent = false
		c.logMemoryUnavailableLocked(err)
		return hostmemory.Reading{}, false
	}
	if !reading.Complete {
		c.memorySamplePresent = false
		c.logMemoryUnavailableLocked(errMemoryReadingIncomplete)
		return hostmemory.Reading{}, false
	}
	c.memorySample = reading
	c.memorySampleAt = now
	c.memorySamplePresent = true
	c.memoryReadFailed = false
	return reading, true
}

// logMemoryUnavailableLocked reports the first failure of a failure streak so
// an operator sees why the guard is inactive without one warning per admission.
func (c *sessionCeilingController) logMemoryUnavailableLocked(err error) {
	if c.memoryReadFailed {
		return
	}
	c.memoryReadFailed = true
	c.logger.Warn("session memory guard could not read host memory; failing open", zap.Error(err))
}

// coordinatorTaskMemoryExemption exempts Office coordinator conversations from
// the host-memory condition. A task that cannot be read is not exempt.
func (s *Service) coordinatorTaskMemoryExemption(ctx context.Context, taskID string) bool {
	if s == nil || s.repo == nil || taskID == "" {
		return false
	}
	task, err := s.repo.GetTask(ctx, taskID)
	return err == nil && task != nil && task.Origin == models.TaskOriginCoordinator
}
