// Package jobs tracks long-running maintenance operations exposed by the
// System pages (VACUUM, factory reset, snapshot create/restore, disk walk,
// etc.) and publishes lifecycle events on the event bus so the WebSocket
// gateway can stream progress to connected clients.
package jobs

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// State is the lifecycle state of a Job.
type State string

const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
)

// Job is a tracked unit of long-running work.
type Job struct {
	ID        string                 `json:"id"`
	Kind      string                 `json:"kind"`
	State     State                  `json:"state"`
	Message   string                 `json:"message,omitempty"`
	Result    map[string]interface{} `json:"result,omitempty"`
	StartedAt time.Time              `json:"started_at"`
	EndedAt   time.Time              `json:"ended_at,omitempty"`
}

// Tracker owns the in-memory job table and publishes lifecycle events.
//
// Tracker is safe for concurrent use.
type Tracker struct {
	mu     sync.RWMutex
	jobs   map[string]*Job
	bus    bus.EventBus
	log    *logger.Logger
	source string
}

// NewTracker constructs a Tracker. busSource is the value placed in the
// event Source field (e.g. "system-jobs").
func NewTracker(eventBus bus.EventBus, log *logger.Logger) *Tracker {
	return &Tracker{
		jobs:   make(map[string]*Job),
		bus:    eventBus,
		log:    log,
		source: "system-jobs",
	}
}

// Start runs fn in a goroutine, publishing queued/running/succeeded|failed
// transitions on the event bus. Returns the job ID immediately. fn returns
// a result map (rendered as part of the succeeded event) or an error
// (which becomes the failed message).
func (t *Tracker) Start(ctx context.Context, kind string, fn func(ctx context.Context) (map[string]interface{}, error)) string {
	job := &Job{
		ID:        uuid.New().String(),
		Kind:      kind,
		State:     StateQueued,
		StartedAt: time.Now().UTC(),
	}

	t.mu.Lock()
	t.jobs[job.ID] = job
	snapshot := *job
	t.mu.Unlock()

	t.publish(ctx, &snapshot)

	go t.run(ctx, job, fn)

	return job.ID
}

func (t *Tracker) run(ctx context.Context, job *Job, fn func(ctx context.Context) (map[string]interface{}, error)) {
	t.transition(ctx, job.ID, func(j *Job) {
		j.State = StateRunning
	})

	result, err := fn(ctx)

	t.transition(ctx, job.ID, func(j *Job) {
		j.EndedAt = time.Now().UTC()
		if err != nil {
			j.State = StateFailed
			j.Message = err.Error()
			return
		}
		j.State = StateSucceeded
		j.Result = result
	})
}

func (t *Tracker) transition(ctx context.Context, id string, mutate func(*Job)) {
	t.mu.Lock()
	job, ok := t.jobs[id]
	if !ok {
		t.mu.Unlock()
		return
	}
	mutate(job)
	snapshot := *job
	t.mu.Unlock()
	t.publish(ctx, &snapshot)
}

func (t *Tracker) publish(ctx context.Context, job *Job) {
	if t.bus == nil {
		return
	}
	if err := t.bus.Publish(ctx, events.SystemJobUpdate, bus.NewEvent(events.SystemJobUpdate, t.source, job)); err != nil && t.log != nil {
		t.log.Warn("failed to publish system.job.update", zap.String("job_id", job.ID), zap.String("kind", job.Kind), zap.Error(err))
	}
}

// Get returns a snapshot of the job by id, or nil if unknown.
func (t *Tracker) Get(id string) *Job {
	t.mu.RLock()
	defer t.mu.RUnlock()
	job, ok := t.jobs[id]
	if !ok {
		return nil
	}
	snapshot := *job
	return &snapshot
}

// List returns snapshots of all tracked jobs.
func (t *Tracker) List() []Job {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Job, 0, len(t.jobs))
	for _, j := range t.jobs {
		out = append(out, *j)
	}
	return out
}
