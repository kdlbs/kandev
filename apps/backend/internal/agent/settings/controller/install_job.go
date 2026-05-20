package controller

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/common/shellexec"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// Install job lifecycle:
//
//   queued  →  running  →  succeeded
//                       ↘  failed
//
// Jobs are kept in memory for jobRetention after they finish so the UI can
// pick up the final state on reload before they evict.

const (
	jobOutputRingSize  = 64 * 1024
	jobRetention       = 5 * time.Minute
	jobMaxParallel     = 4
	jobHardTimeout     = 5 * time.Minute
	outputFlushPeriod  = 50 * time.Millisecond
	outputFlushMaxSize = 4 * 1024
)

// InstallJob tracks a single install attempt.
type InstallJob struct {
	ID         string
	AgentName  string
	Status     dto.InstallJobStatus
	Output     *ringBuffer
	StartedAt  time.Time
	FinishedAt *time.Time
	ExitCode   *int
	Error      string

	cancel context.CancelFunc
}

// streamingInstallRunner exec's the script under a context, streaming each line
// to the provided callback. Swappable in tests.
var streamingInstallRunner = defaultStreamingInstallRunner

func defaultStreamingInstallRunner(
	ctx context.Context,
	script string,
	onChunk func(chunk string),
) error {
	cmd := shellexec.CommandContext(ctx, shellexec.PosixSh, script)
	// Strip npm/pnpm-injected env vars before invoking the install script.
	// When kandev is launched via `pnpm dev`, pnpm sets npm_config_prefix
	// (and friends) to the workspace package directory; if we let those
	// flow through to `npm install -g ...` the package lands in the
	// workspace's bin/ instead of the user's real npm prefix and the
	// freshly installed CLI is invisible to the discovery LookPath check.
	cmd.Env = filteredInstallEnv()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go pumpLines(stdout, onChunk, &wg, errCh)
	go pumpLines(stderr, onChunk, &wg, errCh)
	wg.Wait()
	close(errCh)

	waitErr := cmd.Wait()
	if waitErr != nil {
		return waitErr
	}
	// If cmd.Wait succeeded but a scanner failed (e.g. token too long),
	// surface the read error so the job is marked failed rather than
	// silently succeeded with truncated output.
	for scanErr := range errCh {
		if scanErr != nil {
			return scanErr
		}
	}
	return nil
}

func pumpLines(r io.Reader, onChunk func(string), wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()
	scanner := bufio.NewScanner(r)
	// Lift the default 64KB limit — npm output can have long lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)
	for scanner.Scan() {
		onChunk(scanner.Text() + "\n")
	}
	if err := scanner.Err(); err != nil {
		errCh <- err
	}
}

// JobStore owns active and recently-completed install jobs.
type JobStore struct {
	mu          sync.Mutex
	jobs        map[string]*InstallJob // by job_id
	activeByAgt map[string]*InstallJob // by agent name (current/queued)
	semaphore   chan struct{}          // bounded concurrency
	hub         JobBroadcaster
	log         *zap.Logger
	onSuccess   func(agentName string) // invoked when a job succeeds (used to invalidate discovery)
}

// JobBroadcaster is the subset of the gateway Broadcaster we need.
type JobBroadcaster interface {
	Broadcast(msg *ws.Message)
}

// NewJobStore returns a fresh job store wired to the given broadcaster.
func NewJobStore(hub JobBroadcaster, log *zap.Logger, onSuccess func(string)) *JobStore {
	return &JobStore{
		jobs:        make(map[string]*InstallJob),
		activeByAgt: make(map[string]*InstallJob),
		semaphore:   make(chan struct{}, jobMaxParallel),
		hub:         hub,
		log:         log,
		onSuccess:   onSuccess,
	}
}

// Enqueue starts (or returns an existing) install job for the named agent
// running the given hard-coded script. The script is supplied by the agent
// type, not by the request — no user-supplied shell input.
func (s *JobStore) Enqueue(agentName, script string) *InstallJob {
	s.mu.Lock()
	// Idempotent: a running/queued job for this agent reuses the same job_id.
	if existing, ok := s.activeByAgt[agentName]; ok {
		s.mu.Unlock()
		return existing
	}
	job := &InstallJob{
		ID:        uuid.NewString(),
		AgentName: agentName,
		Status:    dto.InstallJobStatusQueued,
		Output:    newRingBuffer(jobOutputRingSize),
		StartedAt: time.Now().UTC(),
	}
	s.jobs[job.ID] = job
	s.activeByAgt[agentName] = job
	s.mu.Unlock()

	s.broadcast(ws.ActionAgentInstallStarted, job.snapshot())
	go s.run(job, script)
	return job
}

// Get returns a snapshot of the job by ID. ok=false if not found.
func (s *JobStore) Get(jobID string) (*dto.InstallJobDTO, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return nil, false
	}
	snap := job.snapshot()
	return &snap, true
}

// ListAll returns snapshots of every job currently retained (active + recently finished).
func (s *JobStore) ListAll() []dto.InstallJobDTO {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]dto.InstallJobDTO, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j.snapshot())
	}
	return out
}

func (s *JobStore) run(job *InstallJob, script string) {
	// Bound parallel installs: queued jobs wait here until a slot frees up.
	s.semaphore <- struct{}{}
	defer func() { <-s.semaphore }()

	ctx, cancel := context.WithTimeout(context.Background(), jobHardTimeout)
	s.mu.Lock()
	job.cancel = cancel
	job.Status = dto.InstallJobStatusRunning
	s.mu.Unlock()
	defer cancel()

	// Re-broadcast started so the UI flips queued→running.
	s.broadcast(ws.ActionAgentInstallStarted, job.snapshot())

	flusher := newOutputFlusher(s, job)
	defer flusher.flush()

	err := streamingInstallRunner(ctx, script, flusher.append)
	flusher.flush()

	s.mu.Lock()
	now := time.Now().UTC()
	job.FinishedAt = &now
	if err == nil {
		job.Status = dto.InstallJobStatusSucceeded
		exit := 0
		job.ExitCode = &exit
	} else {
		job.Status = dto.InstallJobStatusFailed
		job.Error = formatJobError(ctx, err)
		if exit, ok := exitCodeOf(err); ok {
			job.ExitCode = &exit
		}
	}
	// Free the per-agent slot before notifying so a retry/auto-rescan
	// triggered from onSuccess or the WS broadcast starts a fresh job
	// instead of being deduped against this finished one.
	delete(s.activeByAgt, job.AgentName)
	jobID := job.ID
	s.mu.Unlock()

	if err == nil && s.onSuccess != nil {
		s.onSuccess(job.AgentName)
	}

	s.broadcast(ws.ActionAgentInstallFinished, job.snapshot())

	// Evict the finished job after retention.
	time.AfterFunc(jobRetention, func() {
		s.mu.Lock()
		delete(s.jobs, jobID)
		s.mu.Unlock()
	})
}

func (s *JobStore) broadcast(action string, payload dto.InstallJobDTO) {
	if s.hub == nil {
		return
	}
	msg, _ := ws.NewNotification(action, payload)
	s.hub.Broadcast(msg)
}

func formatJobError(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf("install timed out after %s", jobHardTimeout)
	}
	return err.Error()
}

func exitCodeOf(err error) (int, bool) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), true
	}
	return 0, false
}

// snapshot returns a value copy of the job under the store lock.
// Caller is responsible for holding s.mu *or* calling on a job not yet inserted.
func (j *InstallJob) snapshot() dto.InstallJobDTO {
	snap := dto.InstallJobDTO{
		JobID:     j.ID,
		Name:      j.AgentName,
		Status:    j.Status,
		Output:    j.Output.String(),
		StartedAt: j.StartedAt,
		Error:     j.Error,
	}
	if j.FinishedAt != nil {
		finishedCopy := *j.FinishedAt
		snap.FinishedAt = &finishedCopy
	}
	if j.ExitCode != nil {
		exitCopy := *j.ExitCode
		snap.ExitCode = &exitCopy
	}
	return snap
}

// outputFlusher batches output chunks into broadcasts to limit WS message rate.
type outputFlusher struct {
	store *JobStore
	job   *InstallJob
	mu    sync.Mutex
	buf   bytes.Buffer
	timer *time.Timer
}

func newOutputFlusher(store *JobStore, job *InstallJob) *outputFlusher {
	return &outputFlusher{store: store, job: job}
}

func (f *outputFlusher) append(chunk string) {
	if chunk == "" {
		return
	}
	f.mu.Lock()
	f.buf.WriteString(chunk)
	// Always append to the persistent ring buffer immediately so snapshots are
	// up-to-date; broadcasts are batched. ringBuffer.Write never errs.
	_, _ = f.job.Output.Write([]byte(chunk))
	size := f.buf.Len()
	if size >= outputFlushMaxSize {
		f.mu.Unlock()
		f.flush()
		return
	}
	if f.timer == nil {
		f.timer = time.AfterFunc(outputFlushPeriod, f.flush)
	}
	f.mu.Unlock()
}

func (f *outputFlusher) flush() {
	f.mu.Lock()
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
	if f.buf.Len() == 0 {
		f.mu.Unlock()
		return
	}
	chunk := f.buf.String()
	f.buf.Reset()
	f.mu.Unlock()

	if f.store.hub == nil {
		return
	}
	payload := struct {
		JobID     string `json:"job_id"`
		AgentName string `json:"agent_name"`
		Chunk     string `json:"chunk"`
	}{
		JobID:     f.job.ID,
		AgentName: f.job.AgentName,
		Chunk:     chunk,
	}
	msg, _ := ws.NewNotification(ws.ActionAgentInstallOutput, payload)
	f.store.hub.Broadcast(msg)
}

// ringBuffer is a fixed-size byte buffer that drops the oldest data when full.
// Concurrent Write + String is safe.
type ringBuffer struct {
	mu   sync.Mutex
	data []byte
	cap  int
}

func newRingBuffer(cap int) *ringBuffer {
	return &ringBuffer{cap: cap, data: make([]byte, 0, cap)}
}

func (r *ringBuffer) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = append(r.data, b...)
	if len(r.data) > r.cap {
		excess := len(r.data) - r.cap
		// Compact: drop oldest 'excess' bytes plus enough to land on a line
		// boundary, so the UI doesn't see a half-line.
		nlIdx := bytes.IndexByte(r.data[excess:], '\n')
		if nlIdx >= 0 {
			excess += nlIdx + 1
		}
		if excess > len(r.data) {
			excess = len(r.data)
		}
		r.data = append(r.data[:0], r.data[excess:]...)
	}
	return len(b), nil
}

func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.data)
}

// filteredInstallEnv returns os.Environ() with npm/pnpm-injected configuration
// variables removed. pnpm exports npm_config_prefix (and friends) to scripts
// it spawns; if those leak into a child `npm install -g ...` the package gets
// installed into the workspace package dir instead of the user's global npm
// prefix. Diverges from isNpmEnvVar in agentctl/server/{config,process}: those
// blanket-strip every npm_config_* to silence npx warnings, but install scripts
// genuinely need the user's legitimate npm config (registry, proxy, auth
// tokens, custom .npmrc) to reach private registries or work behind a corporate
// proxy. We only strip the specific keys pnpm injects with workspace-local
// values, plus the per-script context vars that npm install never reads.
func filteredInstallEnv() []string {
	parent := os.Environ()
	out := make([]string, 0, len(parent))
	for _, entry := range parent {
		eq := strings.IndexByte(entry, '=')
		if eq <= 0 {
			out = append(out, entry)
			continue
		}
		if isInstallNpmEnvVar(entry[:eq]) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func isInstallNpmEnvVar(key string) bool {
	switch key {
	case "npm_config_prefix", // pnpm sets to workspace dir; redirects -g installs
		"npm_config_dir",        // same: pnpm-injected workspace dir
		"npm_config_user_agent", // pnpm/X.Y.Z; misleading and harmless to drop
		"npm_execpath",          // path to pnpm binary, not npm
		"npm_node_execpath":     // node binary path from pnpm context
		return true
	}
	// Per-script context, never user config.
	return strings.HasPrefix(key, "npm_package_") ||
		strings.HasPrefix(key, "npm_lifecycle_")
}
