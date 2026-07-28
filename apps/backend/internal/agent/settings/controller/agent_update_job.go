package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type AgentUpdateJob struct {
	ID             string
	AgentName      string
	Status         dto.AgentUpdateJobStatus
	CurrentVersion string
	TargetVersion  string
	Output         *ringBuffer
	StartedAt      time.Time
	FinishedAt     *time.Time
	Error          string
	RefreshError   string
}

type AgentUpdateJobStore struct {
	mu          sync.Mutex
	jobs        map[string]*AgentUpdateJob
	activeByAgt map[string]*AgentUpdateJob
	semaphore   chan struct{}
	hub         JobBroadcaster
	log         *zap.Logger
	updater     RuntimeUpdater
	maintenance *maintenanceCoordinator
	onRefresh   func()
}

func NewAgentUpdateJobStore(
	hub JobBroadcaster,
	log *zap.Logger,
	updater RuntimeUpdater,
	maintenance *maintenanceCoordinator,
	onRefresh func(),
) *AgentUpdateJobStore {
	if maintenance == nil {
		maintenance = newMaintenanceCoordinator()
	}
	return &AgentUpdateJobStore{
		jobs:        make(map[string]*AgentUpdateJob),
		activeByAgt: make(map[string]*AgentUpdateJob),
		semaphore:   make(chan struct{}, jobMaxParallel),
		hub:         hub,
		log:         log,
		updater:     updater,
		maintenance: maintenance,
		onRefresh:   onRefresh,
	}
}

func (s *AgentUpdateJobStore) Enqueue(
	agentName string,
	spec agents.ManagedNPMRuntimeSpec,
) (*AgentUpdateJob, error) {
	s.mu.Lock()
	if existing, ok := s.activeByAgt[agentName]; ok {
		s.mu.Unlock()
		return existing, nil
	}
	job := &AgentUpdateJob{
		ID:        uuid.NewString(),
		AgentName: agentName,
		Status:    dto.AgentUpdateJobStatusQueued,
		Output:    newRingBuffer(jobOutputRingSize),
		StartedAt: time.Now().UTC(),
	}
	ref, claimed, err := s.maintenance.claim(agentName, MaintenanceKindUpdate, job.ID)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	if !claimed {
		existing := s.jobs[ref.JobID]
		s.mu.Unlock()
		return existing, nil
	}
	s.jobs[job.ID] = job
	s.activeByAgt[agentName] = job
	s.mu.Unlock()

	s.broadcast(ws.ActionAgentUpdateStarted, job.snapshot())
	go s.run(job, spec, ref)
	return job, nil
}

func (s *AgentUpdateJobStore) Get(jobID string) (*dto.AgentUpdateJobDTO, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return nil, false
	}
	snapshot := job.snapshot()
	return &snapshot, true
}

func (s *AgentUpdateJobStore) ListAll() []dto.AgentUpdateJobDTO {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]dto.AgentUpdateJobDTO, 0, len(s.jobs))
	for _, job := range s.jobs {
		out = append(out, job.snapshot())
	}
	return out
}

func (s *AgentUpdateJobStore) run(
	job *AgentUpdateJob,
	spec agents.ManagedNPMRuntimeSpec,
	ref MaintenanceJobRef,
) {
	s.semaphore <- struct{}{}
	defer func() { <-s.semaphore }()

	ctx, cancel := context.WithTimeout(context.Background(), jobHardTimeout)
	defer cancel()
	s.setStatus(job, dto.AgentUpdateJobStatusResolving)

	if caps, ok := s.updater.CurrentCapabilities(job.AgentName); ok {
		s.mu.Lock()
		job.CurrentVersion = caps.AgentVersion
		s.mu.Unlock()
	}
	target, err := s.updater.ResolveTarget(ctx, spec.Package)
	if err != nil {
		s.finishFailed(job, ctx, fmt.Errorf("resolve target version: %w", err), ref)
		return
	}
	s.mu.Lock()
	job.TargetVersion = target
	s.mu.Unlock()

	s.setStatus(job, dto.AgentUpdateJobStatusUpdating)
	flusher := newUpdateOutputFlusher(s, job)
	err = s.updater.RunUpdate(ctx, spec.CacheUpdateCommand(), flusher.append)
	flusher.flush()
	if err != nil {
		flusher.append("managed runtime cache appears stale; repairing execution cache\n")
		flusher.flush()
		if repairErr := s.updater.InvalidateExecutionCache(ctx, spec.Package); repairErr != nil {
			s.finishFailed(job, ctx, fmt.Errorf("repair runtime execution cache: %w", repairErr), ref)
			return
		}
		flusher.append("retrying managed runtime update\n")
		flusher.flush()
		err = s.updater.RunUpdate(ctx, spec.CacheUpdateCommand(), flusher.append)
		flusher.flush()
		if err != nil {
			s.finishFailed(job, ctx, fmt.Errorf("update runtime after cache repair: %w", err), ref)
			return
		}
	}

	s.setStatus(job, dto.AgentUpdateJobStatusRefreshing)
	caps, refreshErr := s.updater.Refresh(ctx, job.AgentName, spec.CachedACPCommand())
	s.finishRefresh(job, ctx, caps, refreshErr, ref)
}

func (s *AgentUpdateJobStore) setStatus(job *AgentUpdateJob, status dto.AgentUpdateJobStatus) {
	s.mu.Lock()
	job.Status = status
	snapshot := job.snapshot()
	s.mu.Unlock()
	// A single in-progress action carries every queued-to-active transition so
	// clients can upsert one job snapshot without subscribing to phase-specific
	// actions. Terminal states use ActionAgentUpdateFinished below.
	s.broadcast(ws.ActionAgentUpdateStarted, snapshot)
}

func (s *AgentUpdateJobStore) finishFailed(
	job *AgentUpdateJob,
	ctx context.Context,
	err error,
	ref MaintenanceJobRef,
) {
	s.mu.Lock()
	job.Status = dto.AgentUpdateJobStatusFailed
	job.Error = formatUpdateJobError(ctx, err)
	// Release the shared claim while the local active entry is still present.
	// That makes same-agent retries atomic from the perspective of Enqueue.
	s.maintenance.release(job.AgentName, ref)
	s.finishLocked(job)
	snapshot := job.snapshot()
	s.mu.Unlock()
	s.broadcast(ws.ActionAgentUpdateFinished, snapshot)
	s.scheduleEviction(job.ID)
}

func (s *AgentUpdateJobStore) finishRefresh(
	job *AgentUpdateJob,
	ctx context.Context,
	caps hostutility.AgentCapabilities,
	err error,
	ref MaintenanceJobRef,
) {
	refreshed := false
	s.mu.Lock()
	switch {
	case err != nil:
		job.Status = dto.AgentUpdateJobStatusFailed
		job.Error = formatUpdateJobError(ctx, fmt.Errorf("refresh capabilities: %w", err))
	case caps.Status == hostutility.StatusOK:
		job.Status = dto.AgentUpdateJobStatusSucceeded
		refreshed = true
	case caps.Status == hostutility.StatusAuthRequired:
		job.Status = dto.AgentUpdateJobStatusSucceeded
		job.RefreshError = caps.Error
		if job.RefreshError == "" {
			job.RefreshError = "authentication required"
		}
	default:
		job.Status = dto.AgentUpdateJobStatusFailed
		job.Error = capabilityRefreshError(caps)
	}
	// See finishFailed: a retry must not see a stale shared claim after the
	// local active entry has been removed.
	s.maintenance.release(job.AgentName, ref)
	s.finishLocked(job)
	snapshot := job.snapshot()
	s.mu.Unlock()
	if refreshed && s.onRefresh != nil {
		s.onRefresh()
	}
	s.broadcast(ws.ActionAgentUpdateFinished, snapshot)
	s.scheduleEviction(job.ID)
}

func (s *AgentUpdateJobStore) finishLocked(job *AgentUpdateJob) {
	now := time.Now().UTC()
	job.FinishedAt = &now
	delete(s.activeByAgt, job.AgentName)
}

func capabilityRefreshError(caps hostutility.AgentCapabilities) string {
	if caps.Error != "" {
		return "refresh capabilities: " + caps.Error
	}
	return "refresh capabilities failed with status " + string(caps.Status)
}

func formatUpdateJobError(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Sprintf("update timed out after %s", jobHardTimeout)
	}
	return err.Error()
}

func (s *AgentUpdateJobStore) scheduleEviction(jobID string) {
	time.AfterFunc(jobRetention, func() {
		s.mu.Lock()
		delete(s.jobs, jobID)
		s.mu.Unlock()
	})
}

func (s *AgentUpdateJobStore) broadcast(action string, payload dto.AgentUpdateJobDTO) {
	if s.hub == nil {
		return
	}
	message, _ := ws.NewNotification(action, payload)
	//ws:global Managed runtime updates are host-wide Settings state.
	s.hub.Broadcast(message)
}

func (j *AgentUpdateJob) snapshot() dto.AgentUpdateJobDTO {
	snapshot := dto.AgentUpdateJobDTO{
		JobID:          j.ID,
		AgentName:      j.AgentName,
		Status:         j.Status,
		CurrentVersion: j.CurrentVersion,
		TargetVersion:  j.TargetVersion,
		Output:         j.Output.String(),
		Error:          j.Error,
		RefreshError:   j.RefreshError,
		StartedAt:      j.StartedAt,
	}
	if j.FinishedAt != nil {
		finished := *j.FinishedAt
		snapshot.FinishedAt = &finished
	}
	return snapshot
}

type updateOutputFlusher struct {
	store *AgentUpdateJobStore
	job   *AgentUpdateJob
	mu    sync.Mutex
	buf   bytes.Buffer
	timer *time.Timer
}

func newUpdateOutputFlusher(store *AgentUpdateJobStore, job *AgentUpdateJob) *updateOutputFlusher {
	return &updateOutputFlusher{store: store, job: job}
}

func (f *updateOutputFlusher) append(chunk string) {
	if chunk == "" {
		return
	}
	f.mu.Lock()
	f.buf.WriteString(chunk)
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

func (f *updateOutputFlusher) flush() {
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
	}{JobID: f.job.ID, AgentName: f.job.AgentName, Chunk: chunk}
	message, _ := ws.NewNotification(ws.ActionAgentUpdateOutput, payload)
	//ws:global Managed runtime update output is host-wide Settings state.
	f.store.hub.Broadcast(message)
}
