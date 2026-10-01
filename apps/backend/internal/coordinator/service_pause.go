package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/coordinator/pause"
)

// pauseStopTimeout bounds one Stop run started by a committed pause.
const pauseStopTimeout = 30 * time.Second

// PauserNames resolves the display name of the manager who paused. ok is false
// when the account is gone.
type PauserNames interface {
	UserDisplayName(ctx context.Context, userID string) (name string, ok bool, err error)
}

// WithPhase31 turns the phase 3.1 surface (the pause route) on or off; callers
// pass the effective condition, never the raw flag. The pause gate itself is
// compiled in and ignores it, except for the read-error carve-out.
func WithPhase31(on bool) ServiceOption {
	return func(s *Service) { s.phase31 = on }
}

// Phase31Enabled reports whether the phase 3.1 surface is effective.
func (s *Service) Phase31Enabled() bool { return s.phase31 }

// PauseGate is the precondition in front of every act-on-its-own decision.
func (s *Service) PauseGate() pause.Gate { return s.gate }

// SetPauserNames registers the resolver for the pausing manager's name; nil
// leaves every paused_by null.
func (s *Service) SetPauserNames(r PauserNames) {
	s.pauseMu.Lock()
	defer s.pauseMu.Unlock()
	s.pauserNames = r
}

// LoadKnownPaused fills the known-paused set at boot. A failed read is logged
// and leaves the set empty.
func (s *Service) LoadKnownPaused(ctx context.Context) {
	if err := s.refreshKnownPaused(ctx); err != nil {
		s.logger.Warn("known-paused set could not be loaded", zap.Error(err))
	}
}

func (s *Service) refreshKnownPaused(ctx context.Context) error {
	ids, err := s.store.PausedCoordinatorIDs(ctx)
	if err != nil {
		return err
	}
	s.knownPaused.Replace(ids)
	return nil
}

type pauseRequest struct {
	Paused *bool `json:"paused"`
}

func parsePauseBody(body []byte) (bool, error) {
	var req pauseRequest
	if err := json.Unmarshal(body, &req); err != nil || req.Paused == nil {
		return false, &FieldError{Field: "paused", Message: "paused must be true or false"}
	}
	return *req.Paused, nil
}

// SetPaused backs PUT .../pause. Checks run in order: the flag (404), the
// coordinator (404), the manager scope (403), the body (400), then the
// conditional statement. A committed change publishes coordinator.updated once
// and, for a pause, starts Stop in a service-owned goroutine; the call never
// waits for it.
func (s *Service) SetPaused(ctx context.Context, workspaceID, coordinatorID string, body []byte) (*Coordinator, error) {
	if !s.phase31 {
		return nil, ErrNotFound
	}
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	if _, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID); err != nil {
		return nil, err
	}
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	paused, err := parsePauseBody(body)
	if err != nil {
		return nil, err
	}
	changed, err := s.store.SetPaused(ctx, coordinatorID, paused, decidingUserID(ctx))
	if err != nil {
		return nil, err
	}
	if changed {
		s.afterPauseChange(ctx, workspaceID, coordinatorID, paused)
	}
	return s.store.GetCoordinator(ctx, workspaceID, coordinatorID)
}

func (s *Service) afterPauseChange(ctx context.Context, workspaceID, coordinatorID string, paused bool) {
	s.knownPaused.Set(coordinatorID, paused)
	s.logger.Info("coordinator pause changed",
		zap.String("coordinator_id", coordinatorID), zap.Bool("paused", paused))
	s.publishCoordinatorUpdatedWith(ctx, workspaceID, coordinatorID, true)
	if paused {
		s.runPaused(func(runCtx context.Context) { s.stopPaused(runCtx, coordinatorID) })
	}
}

// stopPaused runs Stop for one coordinator and logs its outcome; a coordinator
// deleted meanwhile ends the run quietly.
func (s *Service) stopPaused(ctx context.Context, coordinatorID string) {
	err := s.Stop(ctx, coordinatorID)
	switch {
	case err == nil:
	case errors.Is(err, ErrNotFound):
		s.logger.Info("coordinator deleted while pause stop ran", zap.String("coordinator_id", coordinatorID))
	default:
		s.logger.Warn("pause stop failed", zap.String("coordinator_id", coordinatorID), zap.Error(err))
	}
}

// pauseRunner owns the goroutines a committed pause and the late-send path
// start: one detached context bounded per run, drained on shutdown.
type pauseRunner struct {
	mu      sync.Mutex
	wg      sync.WaitGroup
	base    context.Context
	cancel  context.CancelFunc
	stopped bool
}

// runPaused runs fn in a service-owned goroutine with a detached 30 second
// context. After StopPause it runs nothing.
func (s *Service) runPaused(fn func(ctx context.Context)) {
	r := &s.pauseRun
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	if r.base == nil {
		r.base, r.cancel = context.WithCancel(context.Background())
	}
	base := r.base
	r.wg.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.wg.Done()
		ctx, cancel := context.WithTimeout(base, pauseStopTimeout)
		defer cancel()
		fn(ctx)
	}()
}

// StopPause cancels and drains the goroutines started by Pause. Idempotent.
func (s *Service) StopPause() {
	r := &s.pauseRun
	r.mu.Lock()
	r.stopped = true
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
}

// PauseView is the paused part of the coordinator and autonomy reads.
type PauseView struct {
	Paused   bool         `json:"paused"`
	PausedAt *string      `json:"paused_at"`
	PausedBy *PausedByDTO `json:"paused_by"`
}

// PausedByDTO names the manager who paused.
type PausedByDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// pauseView renders the stored state. paused_by is null when the stored user
// id is empty, the user is gone or the name is not readable.
func (s *Service) pauseView(ctx context.Context, c *Coordinator) PauseView {
	if c.PausedAt == nil {
		return PauseView{}
	}
	view := PauseView{Paused: true, PausedAt: timeStrPtr(c.PausedAt)}
	if c.PausedBy == "" {
		return view
	}
	s.pauseMu.Lock()
	names := s.pauserNames
	s.pauseMu.Unlock()
	if names == nil {
		return view
	}
	name, ok, err := names.UserDisplayName(ctx, c.PausedBy)
	if err != nil {
		s.logger.Warn("pausing manager name unreadable", zap.String("coordinator_id", c.ID), zap.Error(err))
		return view
	}
	if ok {
		view.PausedBy = &PausedByDTO{ID: c.PausedBy, Name: name}
	}
	return view
}
