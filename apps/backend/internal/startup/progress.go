// Package startup reports process initialization without exposing private diagnostics.
package startup

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

type Phase string

const (
	OpeningDatabase      Phase = "opening_database"
	BackingUpDatabase    Phase = "backing_up_database"
	ApplyingMigrations   Phase = "applying_migrations"
	InitializingServices Phase = "initializing_services"
	RecoveringSessions   Phase = "recovering_sessions"
	Ready                Phase = "ready"
)

// Label accepts only protocol-owned phase names, never arbitrary server text.
func (p Phase) Label() string {
	switch p {
	case OpeningDatabase:
		return "Opening database"
	case BackingUpDatabase:
		return "Backing up database"
	case ApplyingMigrations:
		return "Applying migrations"
	case InitializingServices:
		return "Initializing services"
	case RecoveringSessions:
		return "Recovering sessions"
	case Ready:
		return "Ready"
	default:
		return ""
	}
}

type Snapshot struct {
	Phase          Phase `json:"phase"`
	ElapsedMS      int64 `json:"elapsed_ms"`
	PhaseElapsedMS int64 `json:"phase_elapsed_ms"`
}

type Reporter struct {
	mu                    sync.Mutex
	started, phaseStarted time.Time
	phase                 Phase
	log                   *logger.Logger
}

type contextKey struct{}

func New(log *logger.Logger) *Reporter {
	now := time.Now()
	return &Reporter{started: now, phaseStarted: now, phase: OpeningDatabase, log: log}
}
func WithReporter(ctx context.Context, r *Reporter) context.Context {
	return context.WithValue(ctx, contextKey{}, r)
}
func FromContext(ctx context.Context) *Reporter {
	r, _ := ctx.Value(contextKey{}).(*Reporter)
	return r
}
func SetPhase(ctx context.Context, phase Phase) {
	if r := FromContext(ctx); r != nil {
		r.Set(phase)
	}
}
func (r *Reporter) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Snapshot{Phase: r.phase, ElapsedMS: time.Since(r.started).Milliseconds(), PhaseElapsedMS: time.Since(r.phaseStarted).Milliseconds()}
}
func (r *Reporter) Set(phase Phase) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.phase == phase {
		return
	}
	if r.log != nil {
		r.log.Info("Startup phase completed", zap.String("phase", string(r.phase)), zap.Duration("duration", time.Since(r.phaseStarted)), zap.Duration("elapsed", time.Since(r.started)))
	}
	r.phase = phase
	r.phaseStarted = time.Now()
	if r.log != nil {
		r.log.Info("Startup phase started", zap.String("phase", string(phase)), zap.Duration("elapsed", time.Since(r.started)))
	}
}
func (r *Reporter) Report() {
	s := r.Snapshot()
	if r.log != nil && s.Phase != Ready {
		r.log.Info("Startup in progress", zap.String("phase", string(s.Phase)), zap.Int64("elapsed_ms", s.ElapsedMS), zap.Int64("phase_elapsed_ms", s.PhaseElapsedMS))
	}
}
