package improvekandev

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/auth/authn"
)

const (
	bootstrapJobTimeout   = 6 * time.Minute
	bootstrapJobRetention = 10 * time.Minute
	bootstrapJobCapacity  = 64
)

var errBootstrapJobsFull = errors.New("too many bootstrap jobs are active")

type bootstrapJobOwner struct {
	userID    string
	orgID     string
	synthetic bool
	instance  bool
}

func ownerForIdentity(identity authn.Identity) bootstrapJobOwner {
	return bootstrapJobOwner{
		userID:    identity.UserID,
		orgID:     identity.OrgID,
		synthetic: identity.Synthetic,
		instance:  identity.Instance,
	}
}

type bootstrapJob struct {
	owner       bootstrapJobOwner
	createdAt   time.Time
	finishedAt  time.Time
	done        bool
	response    *BootstrapResponse
	publicError string
	cancel      context.CancelFunc
}

type bootstrapJobSnapshot struct {
	pending     bool
	response    *BootstrapResponse
	publicError string
}

type bootstrapJobManager struct {
	mu      sync.Mutex
	jobs    map[string]*bootstrapJob
	ctx     context.Context
	cancel  context.CancelFunc
	workers sync.WaitGroup
	stopped bool
}

func newBootstrapJobManager() *bootstrapJobManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &bootstrapJobManager{jobs: make(map[string]*bootstrapJob), ctx: ctx, cancel: cancel}
}

func (m *bootstrapJobManager) start(
	identity authn.Identity,
	prepare func(context.Context) (*BootstrapResponse, string),
) (string, error) {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return "", errors.New("bootstrap service is stopped")
	}
	m.pruneLocked(now)
	if len(m.jobs) >= bootstrapJobCapacity {
		m.evictOldestFinishedLocked()
	}
	if len(m.jobs) >= bootstrapJobCapacity {
		return "", errBootstrapJobsFull
	}

	id := uuid.NewString()
	parent := authn.WithIdentity(m.ctx, identity)
	ctx, cancel := context.WithTimeout(parent, bootstrapJobTimeout)
	m.jobs[id] = &bootstrapJob{owner: ownerForIdentity(identity), createdAt: now, cancel: cancel}
	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		response, publicError := prepare(ctx)
		if publicError != "" {
			response = nil
		} else if ctx.Err() != nil && response == nil {
			publicError = "bootstrap preparation was interrupted"
		}
		cancel()
		m.finish(id, response, publicError)
	}()
	return id, nil
}

func (m *bootstrapJobManager) get(id string, owner bootstrapJobOwner) (bootstrapJobSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked(time.Now())
	job, ok := m.jobs[id]
	if !ok || job.owner != owner {
		return bootstrapJobSnapshot{}, false
	}
	snapshot := bootstrapJobSnapshot{pending: !job.done, publicError: job.publicError}
	if job.response != nil {
		response := *job.response
		snapshot.response = &response
	}
	return snapshot, true
}

func (m *bootstrapJobManager) finish(id string, response *BootstrapResponse, publicError string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return
	}
	job.done = true
	job.finishedAt = time.Now()
	job.cancel = nil
	job.response = response
	job.publicError = publicError
}

func (m *bootstrapJobManager) stop() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		m.workers.Wait()
		return
	}
	m.stopped = true
	m.cancel()
	for _, job := range m.jobs {
		if job.cancel != nil {
			job.cancel()
		}
	}
	m.mu.Unlock()
	m.workers.Wait()
}

func (m *bootstrapJobManager) pruneLocked(now time.Time) {
	for id, job := range m.jobs {
		if job.done && now.Sub(job.finishedAt) >= bootstrapJobRetention {
			delete(m.jobs, id)
		}
	}
}

func (m *bootstrapJobManager) evictOldestFinishedLocked() {
	oldestID := ""
	var oldest time.Time
	for id, job := range m.jobs {
		if job.done && (oldestID == "" || job.finishedAt.Before(oldest)) {
			oldestID = id
			oldest = job.finishedAt
		}
	}
	if oldestID != "" {
		delete(m.jobs, oldestID)
	}
}
