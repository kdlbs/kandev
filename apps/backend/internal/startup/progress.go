// Package startup reports process initialization without exposing private diagnostics.
package startup

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
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
	Phase          Phase                   `json:"phase"`
	Boot           int64                   `json:"boot"`
	Seq            uint64                  `json:"seq"`
	ElapsedMS      int64                   `json:"elapsed_ms"`
	PhaseElapsedMS int64                   `json:"phase_elapsed_ms"`
	Step           *StepSnapshot           `json:"step,omitempty"`
	CompletedSteps []CompletedStepSnapshot `json:"completed_steps,omitempty"`
}

type Reporter struct {
	mu                    sync.Mutex
	started, phaseStarted time.Time
	phase                 Phase
	log                   *logger.Logger
	activations           []func() error
	activated             bool
	activationComplete    bool
	activationDone        chan struct{}
	activationErr         error
	readyAt               time.Time
	readyElapsedMS        int64
	completedSteps        []CompletedStepSnapshot

	boot               int64
	seq                uint64
	active             *stepActivation
	unregisteredWarned map[StepID]bool
	mismatchWarned     map[mismatchKey]bool
}

type contextKey struct{}

func New(log *logger.Logger) *Reporter {
	now := time.Now()
	return &Reporter{started: now, phaseStarted: now, phase: OpeningDatabase, log: log, boot: newBoot(), activationDone: make(chan struct{})}
}

// newBoot draws a positive, non-guessable, per-process identifier below 2^53
// so every value decodes exactly when a consumer reads JSON numbers as
// doubles. crypto/rand.Read never errors on the Go version this module
// requires, which is what lets New keep its error-free signature.
func newBoot() int64 {
	var buf [8]byte
	for {
		_, _ = cryptorand.Read(buf[:])
		v := int64(binary.BigEndian.Uint64(buf[:]) & ((1 << 53) - 1))
		if v != 0 {
			return v
		}
	}
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
	now := time.Now()
	if !r.readyAt.IsZero() {
		now = r.readyAt
	}
	snap := Snapshot{
		Phase:          r.phase,
		Boot:           r.boot,
		Seq:            r.seq,
		ElapsedMS:      now.Sub(r.started).Milliseconds(),
		PhaseElapsedMS: now.Sub(r.phaseStarted).Milliseconds(),
	}
	if r.active != nil {
		snap.Step = r.active.snapshot(now)
	}
	if !r.readyAt.IsZero() {
		snap.ElapsedMS = r.readyElapsedMS
	}
	snap.CompletedSteps = append([]CompletedStepSnapshot(nil), r.completedSteps...)
	return snap
}
func (r *Reporter) Set(phase Phase) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.phase == phase || r.phase == Ready {
		return
	}
	if r.log != nil {
		r.log.Info("Startup phase completed", zap.String("phase", string(r.phase)), zap.Duration("duration", time.Since(r.phaseStarted)), zap.Duration("elapsed", time.Since(r.started)))
	}
	if r.active != nil && r.active.spec.Phase != phase {
		r.endActiveLocked(false)
		r.seq++
	}
	r.phase = phase
	r.phaseStarted = time.Now()
	if phase == Ready {
		r.readyAt = r.phaseStarted
		r.readyElapsedMS = r.readyAt.Sub(r.started).Milliseconds()
	}
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
