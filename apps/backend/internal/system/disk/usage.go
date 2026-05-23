// Package disk computes a lazy, cached breakdown of the on-disk kandev data
// footprint. Service exposes Get (returns cached value, kicks async refresh
// when missing or stale) and Refresh (forces a recompute). The walk runs as
// a tracked job so the WebSocket gateway streams progress to clients.
package disk

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/system/jobs"
)

// cacheTTL controls how long a cached breakdown is considered fresh. After
// it expires, the next Get returns the stale value AND kicks a background
// refresh.
const cacheTTL = 2 * time.Hour

// jobKind is the Kind string used when tracking disk-walk jobs.
const jobKind = "disk-walk"

// Breakdown is the per-subdir disk footprint plus aggregated total and any
// warnings collected from permission errors during the walk. ComputedAt is
// the wall-clock time the walk finished.
type Breakdown struct {
	DataDir    int64     `json:"data_dir"`
	Worktrees  int64     `json:"worktrees"`
	Repos      int64     `json:"repos"`
	Sessions   int64     `json:"sessions"`
	Tasks      int64     `json:"tasks"`
	QuickChat  int64     `json:"quick_chat"`
	Backups    int64     `json:"backups"`
	Total      int64     `json:"total"`
	Warnings   []string  `json:"warnings,omitempty"`
	ComputedAt time.Time `json:"computed_at"`
}

// GetResult is what Service.Get returns: the cached breakdown (or nil when
// none has been computed yet) plus a Computing flag that signals "a
// background walk is in flight, expect a fresh value soon". HomeDir is the
// absolute path the breakdown was computed against, so the UI can show
// "where" the data lives.
type GetResult struct {
	Data      *Breakdown `json:"data"`
	Computing bool       `json:"computing"`
	HomeDir   string     `json:"home_dir"`
}

// Service owns the in-memory cache and coordinates lazy refresh through the
// shared jobs.Tracker. It is safe for concurrent use.
type Service struct {
	homeDir string
	jobs    *jobs.Tracker
	log     *logger.Logger

	mu          sync.Mutex
	value       *Breakdown
	computing   bool
	activeJobID string
}

// NewService constructs a Service for the given home directory. The home
// directory should be cfg.ResolvedHomeDir().
func NewService(homeDir string, tracker *jobs.Tracker, log *logger.Logger) *Service {
	return &Service{
		homeDir: homeDir,
		jobs:    tracker,
		log:     log,
	}
}

// Get returns the current cached breakdown and may kick off an async walk:
//   - cold (no value, not computing) -> start walk, return {nil, true}
//   - cold (no value, computing)     -> return {nil, true}
//   - fresh (value, < TTL)           -> return {value, false}
//   - stale (value, >= TTL)          -> start refresh, return {value, true}
func (s *Service) Get(ctx context.Context) GetResult {
	s.mu.Lock()
	value := s.value
	computing := s.computing
	stale := value != nil && time.Since(value.ComputedAt) >= cacheTTL
	shouldKick := !computing && (value == nil || stale)
	if shouldKick {
		s.computing = true
	}
	s.mu.Unlock()

	if shouldKick {
		s.startWalk(ctx)
	}

	if value == nil {
		return GetResult{Data: nil, Computing: true, HomeDir: s.homeDir}
	}
	return GetResult{Data: value, Computing: computing || stale, HomeDir: s.homeDir}
}

// HomeDir returns the absolute path the disk-usage service walks. Exposed so
// the "open folder" handler can reveal it in the host OS file explorer.
func (s *Service) HomeDir() string {
	return s.homeDir
}

// Refresh kicks off a walk and returns the job ID. If a walk is already in
// flight the in-flight job ID is returned and no new walk is started — a
// concurrent recompute would race on the cache and waste IO.
func (s *Service) Refresh(ctx context.Context) string {
	s.mu.Lock()
	if s.computing && s.activeJobID != "" {
		id := s.activeJobID
		s.mu.Unlock()
		return id
	}
	s.computing = true
	s.mu.Unlock()
	return s.kickJob(ctx)
}

// startWalk schedules an async walk; intended for Get's lazy path where we
// only want the side-effect (no job ID returned).
func (s *Service) startWalk(ctx context.Context) {
	_ = s.kickJob(ctx)
}

// kickJob hands the walk off to the jobs.Tracker so progress flows through
// the standard system.job.update channel. The fn updates the cache before
// returning so observers reading Service state after the job completes see
// the fresh value. The active job ID is tracked so concurrent Refresh
// callers can be coalesced onto the in-flight walk.
func (s *Service) kickJob(ctx context.Context) string {
	id := s.jobs.Start(ctx, jobKind, func(jobCtx context.Context) (map[string]interface{}, error) {
		breakdown := s.computeBreakdown(jobCtx)
		s.storeBreakdown(breakdown)
		return map[string]interface{}{"total_bytes": breakdown.Total}, nil
	})
	s.mu.Lock()
	s.activeJobID = id
	s.mu.Unlock()
	return id
}

// computeBreakdown walks every configured subdir, aggregating sizes and
// warnings into a fresh Breakdown stamped with the wall-clock completion
// time.
func (s *Service) computeBreakdown(_ context.Context) *Breakdown {
	b := &Breakdown{}
	for _, sd := range subdirsFor(s.homeDir) {
		res := walkSubdir(sd.path)
		assignSubdir(b, sd.name, res.bytes)
		b.Total += res.bytes
		if len(res.warnings) > 0 {
			b.Warnings = append(b.Warnings, res.warnings...)
		}
	}
	b.ComputedAt = time.Now().UTC()
	if s.log != nil {
		s.log.Debug("disk walk completed",
			zap.Int64("total_bytes", b.Total),
			zap.Int("warnings", len(b.Warnings)),
		)
	}
	return b
}

// assignSubdir routes the per-subdir size into the matching Breakdown
// field. Kept as a small switch so adding a new subdir is a one-liner here
// plus one line in subdirsFor.
func assignSubdir(b *Breakdown, name string, size int64) {
	switch name {
	case "data_dir":
		b.DataDir = size
	case "worktrees":
		b.Worktrees = size
	case "repos":
		b.Repos = size
	case "sessions":
		b.Sessions = size
	case "tasks":
		b.Tasks = size
	case "quick_chat":
		b.QuickChat = size
	case "backups":
		b.Backups = size
	}
}

// storeBreakdown publishes a freshly computed breakdown into the cache and
// clears the computing flag.
func (s *Service) storeBreakdown(b *Breakdown) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value = b
	s.computing = false
	s.activeJobID = ""
}

// setComputedAt is a test-only helper that rewinds the cached value's
// ComputedAt so tests can observe the TTL-expired branch of Get without
// sleeping for two hours.
func (s *Service) setComputedAt(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.value != nil {
		s.value.ComputedAt = t
	}
}
