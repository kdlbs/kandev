package dream

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/replay"
)

// Failure reasons of a dream; the set is closed.
const (
	ReasonBadOutput   = "bad_output"
	ReasonTimeout     = "timeout"
	ReasonLeaseLost   = "lease_lost"
	ReasonCeiling     = "ceiling"
	ReasonRunError    = "run_error"
	ReasonStoreError  = "store_error"
	ReasonAutonomyOff = "autonomy_off"
	ReasonPaused      = "paused"
	ReasonContainment = "containment"
)

// Skip reason of a row stored when the evidence did not change.
const ReasonUnchanged = "unchanged"

// Scheduler limits.
const (
	MaxReplays        = 5
	MinTurns          = 5
	MinDecisions      = 1
	LeaseExpiry       = 5 * time.Minute
	DefaultBound      = 20 * time.Minute
	DefaultRefresh    = time.Minute
	maxWindow         = 30 * 24 * time.Hour
	defaultBatchLimit = 200
)

// Episode runs the one-tool, no-write agent conversation. Its calls are made in
// order and each id is stored on the dream row before the next call.
type Episode interface {
	CreateTask(ctx context.Context, c *coordinator.Coordinator) (taskID string, err error)
	CreateSession(ctx context.Context, taskID string) (sessionID string, err error)
	// Prompt sends the message and returns the agent's last message.
	Prompt(ctx context.Context, taskID, sessionID, message string) (answer string, err error)
	// Archive archives the task, stopping its running turn; a missing task is archived.
	Archive(ctx context.Context, taskID string) error
}

// Replayer is the replay harness.
type Replayer interface {
	Run(ctx context.Context, req replay.Request) (replay.Result, error)
}

// Conditions are the admission inputs that live outside the store.
type Conditions interface {
	// ContainmentOK reports whether every containment condition holds.
	ContainmentOK(ctx context.Context, c *coordinator.Coordinator) bool
	// Spend reports whether spend is measurable and whether it is at the ceiling.
	Spend(ctx context.Context, c *coordinator.Coordinator, now time.Time) (measurable, atCeiling bool)
	// Model names the model the episode runs on.
	Model(ctx context.Context, c *coordinator.Coordinator) string
}

// Deps are the scheduler's dependencies.
type Deps struct {
	Store      *coordinator.Store
	Episode    Episode
	Replay     Replayer
	Conditions Conditions
	Log        *zap.Logger
	Clock      func() time.Time
	Bound      time.Duration
	Refresh    time.Duration
}

// Scheduler admits, runs and cleans up dreams. Tick is called from the wake
// backstop pass; each dream runs on its own goroutine until Stop.
type Scheduler struct {
	d Deps

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	closed  bool
	wg      sync.WaitGroup
}

// New returns a Scheduler; zero Clock, Bound and Refresh take their defaults.
func New(d Deps) *Scheduler {
	if d.Clock == nil {
		d.Clock = func() time.Time { return time.Now().UTC() }
	}
	if d.Bound <= 0 {
		d.Bound = DefaultBound
	}
	if d.Refresh <= 0 {
		d.Refresh = DefaultRefresh
	}
	if d.Log == nil {
		d.Log = zap.NewNop()
	}
	return &Scheduler{d: d, cancels: map[string]context.CancelFunc{}}
}

// Stop cancels every running dream goroutine and waits for them. Idempotent.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	s.closed = true
	for _, cancel := range s.cancels {
		cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

// Cancel cancels the in-process goroutine of one dream.
func (s *Scheduler) Cancel(dreamID string) {
	s.mu.Lock()
	cancel := s.cancels[dreamID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Tick expires stale leases, cleans up episodes, and starts a dream when every
// admission condition holds. It returns after starting the goroutine.
func (s *Scheduler) Tick(ctx context.Context, coordinatorID string) {
	st := s.d.Store
	c, err := st.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		if !errors.Is(err, coordinator.ErrNotFound) {
			s.d.Log.Warn("dream tick: read coordinator", zap.Error(err))
		}
		return
	}
	now := s.d.Clock()
	expired, err := st.ExpireStaleDreams(ctx, coordinatorID, now.Add(-LeaseExpiry), now)
	if err != nil {
		s.d.Log.Warn("dream tick: expire", zap.Error(err))
	}
	for _, d := range expired {
		s.Cancel(d.ID)
	}
	if _, err := st.SettleStaleReplays(ctx, now); err != nil {
		s.d.Log.Warn("dream tick: settle replays", zap.Error(err))
	}
	s.cleanup(ctx, coordinatorID)
	plan, ok := s.admit(ctx, c, now)
	if !ok {
		return
	}
	s.start(ctx, c, plan)
}

// cleanup archives the episode task of every non-running dream row. It is the
// one place that archives, and it runs whatever the Pause state is.
func (s *Scheduler) cleanup(ctx context.Context, coordinatorID string) {
	rows, err := s.d.Store.DreamsWithOpenEpisode(ctx, coordinatorID)
	if err != nil {
		s.d.Log.Warn("dream tick: open episodes", zap.Error(err))
		return
	}
	for _, r := range rows {
		if r.Status == coordinator.DreamRunning {
			continue
		}
		if err := s.d.Episode.Archive(ctx, r.EpisodeTaskID); err != nil {
			s.d.Log.Warn("dream cleanup: archive", zap.String("dream_id", r.ID), zap.Error(err))
			continue
		}
		if err := s.d.Store.MarkDreamEpisodeArchived(ctx, r.ID, s.d.Clock()); err != nil {
			s.d.Log.Warn("dream cleanup: mark", zap.String("dream_id", r.ID), zap.Error(err))
		}
	}
}

// Stop of a paused coordinator: marks its running row and cancels the episode.
// The tick cleanup archives the task afterwards.
func (s *Scheduler) StopCoordinator(ctx context.Context, coordinatorID string) error {
	row, err := s.d.Store.RunningDream(ctx, coordinatorID)
	if err != nil || row == nil {
		return err
	}
	if _, err := s.d.Store.FailDream(ctx, row.ID, ReasonPaused, s.d.Clock()); err != nil {
		return err
	}
	s.Cancel(row.ID)
	return nil
}

func newID() string { return uuid.NewString() }
