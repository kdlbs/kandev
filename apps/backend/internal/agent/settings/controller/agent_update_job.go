package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type AgentUpdateJob struct {
	ID               string
	AgentName        string
	Package          string
	Status           dto.AgentUpdateJobStatus
	Operation        managedruntime.Operation
	CurrentVersion   string
	DefaultVersion   string
	ActiveVersion    string
	EffectiveVersion string
	TargetVersion    string
	UseDefault       bool
	Output           *ringBuffer
	StartedAt        time.Time
	FinishedAt       *time.Time
	Error            string
	RefreshError     string
}

type AgentUpdateJobStore struct {
	mu                  sync.Mutex
	jobs                map[string]*AgentUpdateJob
	activeByAgt         map[string]*AgentUpdateJob
	semaphore           chan struct{}
	hub                 JobBroadcaster
	log                 *zap.Logger
	updater             RuntimeUpdater
	maintenance         *maintenanceCoordinator
	onRefresh           func()
	selectionStore      managedruntime.SelectionStore
	onStatusInvalidated func(string)
}

func NewAgentUpdateJobStore(
	hub JobBroadcaster,
	log *zap.Logger,
	updater RuntimeUpdater,
	maintenance *maintenanceCoordinator,
	onRefresh func(),
	selectionStores ...managedruntime.SelectionStore,
) *AgentUpdateJobStore {
	if maintenance == nil {
		maintenance = newMaintenanceCoordinator()
	}
	var selectionStore managedruntime.SelectionStore
	if len(selectionStores) > 0 {
		selectionStore = selectionStores[0]
	}
	return &AgentUpdateJobStore{
		jobs:           make(map[string]*AgentUpdateJob),
		activeByAgt:    make(map[string]*AgentUpdateJob),
		semaphore:      make(chan struct{}, jobMaxParallel),
		hub:            hub,
		log:            log,
		updater:        updater,
		maintenance:    maintenance,
		onRefresh:      onRefresh,
		selectionStore: selectionStore,
	}
}

func (s *AgentUpdateJobStore) Enqueue(
	agentName string,
	spec agents.ManagedNPMRuntimeSpec,
	targetVersions ...string,
) (*AgentUpdateJob, error) {
	return s.enqueue(agentName, spec, false, targetVersions...)
}

func (s *AgentUpdateJobStore) EnqueueDefault(
	agentName string,
	spec agents.ManagedNPMRuntimeSpec,
) (*AgentUpdateJob, error) {
	return s.enqueue(agentName, spec, true)
}

func (s *AgentUpdateJobStore) enqueue(
	agentName string,
	spec agents.ManagedNPMRuntimeSpec,
	useDefault bool,
	targetVersions ...string,
) (*AgentUpdateJob, error) {
	s.mu.Lock()
	if existing, ok := s.activeByAgt[agentName]; ok {
		s.mu.Unlock()
		return existing, nil
	}
	job := &AgentUpdateJob{
		ID:         uuid.NewString(),
		AgentName:  agentName,
		Package:    spec.Package,
		Status:     dto.AgentUpdateJobStatusQueued,
		Output:     newRingBuffer(jobOutputRingSize),
		StartedAt:  time.Now().UTC(),
		UseDefault: useDefault,
	}
	requestedTarget := ""
	if len(targetVersions) > 0 {
		requestedTarget = strings.TrimSpace(targetVersions[0])
		job.TargetVersion = requestedTarget
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
	go s.run(job, spec, requestedTarget, ref)
	return job, nil
}

// SetStatusInvalidator wires the process-local status cache invalidation used
// after a successful runtime activation. It is optional for isolated stores.
func (s *AgentUpdateJobStore) SetStatusInvalidator(invalidator func(string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStatusInvalidated = invalidator
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

// GetActive returns the current queued or running update for an agent.
func (s *AgentUpdateJobStore) GetActive(agentName string) (*dto.AgentUpdateJobDTO, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.activeByAgt[agentName]
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
	requestedTarget string,
	ref MaintenanceJobRef,
) {
	s.semaphore <- struct{}{}
	defer func() { <-s.semaphore }()

	ctx, cancel := context.WithTimeout(context.Background(), jobHardTimeout)
	defer cancel()
	s.setStatus(job, dto.AgentUpdateJobStatusResolving)

	var (
		target      string
		exactTarget bool
		err         error
	)
	if job.UseDefault {
		target = spec.DefaultVersionOrPinned()
		if _, parseErr := managedruntime.ParseStableVersion(target); parseErr != nil {
			err = fmt.Errorf("resolve default version: %w", parseErr)
		}
		exactTarget = true
	} else {
		target, exactTarget, err = s.resolveTarget(ctx, spec.Package, requestedTarget)
	}
	if err != nil {
		s.finishFailed(job, ctx, fmt.Errorf("resolve target version: %w", err), ref)
		return
	}
	currentVersion := ""
	if caps, ok := s.updater.CurrentCapabilities(job.AgentName); ok {
		currentVersion = caps.AgentVersion
		s.mu.Lock()
		job.CurrentVersion = currentVersion
		s.mu.Unlock()
	}
	activeVersion := ""
	if s.selectionStore != nil {
		selection, found, selectionErr := s.selectionStore.Get(ctx, job.AgentName, spec.Package)
		if selectionErr != nil {
			s.finishFailed(job, ctx, fmt.Errorf("read active runtime version: %w", selectionErr), ref)
			return
		}
		if found {
			activeVersion = selection.Version
		}
	}
	defaultVersion := spec.DefaultVersionOrPinned()
	effectiveVersion := defaultVersion
	if activeVersion != "" {
		effectiveVersion = activeVersion
	}
	operation, err := managedruntime.ClassifyEffectiveOperation(
		job.UseDefault, activeVersion, effectiveVersion, currentVersion, target, defaultVersion,
	)
	if err != nil {
		s.finishFailed(job, ctx, fmt.Errorf("classify runtime operation: %w", err), ref)
		return
	}
	s.mu.Lock()
	job.TargetVersion = target
	job.Operation = operation
	job.DefaultVersion = defaultVersion
	job.ActiveVersion = activeVersion
	job.EffectiveVersion = effectiveVersion
	s.mu.Unlock()
	if operation == managedruntime.OperationUpToDate ||
		(s.selectionStore == nil && currentVersion != "" && currentVersion == target) {
		s.finishAlreadyUpToDate(job, ref)
		return
	}
	if candidate, ok := s.updater.(RuntimeCandidateUpdater); ok && s.selectionStore != nil {
		s.runExactCandidate(ctx, job, spec, target, candidate, ref)
		return
	}

	s.setStatus(job, dto.AgentUpdateJobStatusUpdating)
	flusher := newUpdateOutputFlusher(s, job)
	prepareCommand := spec.CacheUpdateCommand()
	if exactTarget {
		prepareCommand = spec.CacheUpdateCommand(target)
	}
	err = s.updater.RunUpdate(ctx, prepareCommand, flusher.append)
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
		err = s.updater.RunUpdate(ctx, prepareCommand, flusher.append)
		flusher.flush()
		if err != nil {
			s.finishFailed(job, ctx, fmt.Errorf("update runtime after cache repair: %w", err), ref)
			return
		}
	}

	s.setStatus(job, dto.AgentUpdateJobStatusRefreshing)
	refreshCommand := spec.CachedACPCommand()
	if exactTarget {
		refreshCommand = spec.ACPCommand(target)
	}
	caps, refreshErr := s.updater.Refresh(ctx, job.AgentName, refreshCommand)
	s.finishRefresh(job, ctx, caps, refreshErr, ref)
}

func (s *AgentUpdateJobStore) resolveTarget(
	ctx context.Context,
	packageName string,
	requestedTarget string,
) (string, bool, error) {
	if resolver, ok := s.updater.(RuntimeVersionResolver); ok {
		return s.resolveTargetFromMetadata(ctx, resolver, packageName, requestedTarget)
	}
	if requestedTarget != "" {
		if _, err := managedruntime.ParseStableVersion(requestedTarget); err != nil {
			return "", true, fmt.Errorf("%w: %v", ErrRuntimeUpdateTargetInvalid, err)
		}
		return requestedTarget, true, nil
	}
	target, err := s.updater.ResolveTarget(ctx, packageName)
	return target, false, err
}

func (s *AgentUpdateJobStore) resolveTargetFromMetadata(
	ctx context.Context,
	resolver RuntimeVersionResolver,
	packageName string,
	requestedTarget string,
) (string, bool, error) {
	requestedTarget = strings.TrimSpace(requestedTarget)
	if requestedTarget != "" {
		// EnqueueAgentUpdate already checked this exact target against the
		// package catalogue. The worker only needs to preserve that target,
		// avoiding a second registry round-trip after the job is queued.
		if _, err := managedruntime.ParseStableVersion(requestedTarget); err != nil {
			return "", true, fmt.Errorf("%w: %v", ErrRuntimeUpdateTargetInvalid, err)
		}
		return requestedTarget, true, nil
	}
	metadata, err := resolver.ResolveVersions(ctx, packageName)
	if err != nil {
		return "", true, err
	}
	catalogue, err := managedruntime.BuildCatalogue(metadata.Versions, metadata.Latest)
	if err != nil {
		return "", true, err
	}
	return catalogue.Latest, true, nil
}

func (s *AgentUpdateJobStore) runExactCandidate(
	ctx context.Context,
	job *AgentUpdateJob,
	spec agents.ManagedNPMRuntimeSpec,
	target string,
	candidate RuntimeCandidateUpdater,
	ref MaintenanceJobRef,
) {
	s.setStatus(job, dto.AgentUpdateJobStatusUpdating)
	flusher := newUpdateOutputFlusher(s, job)
	prepareCommand := spec.CacheUpdateCommand(target)
	err := s.updater.RunUpdate(ctx, prepareCommand, flusher.append)
	flusher.flush()
	if err != nil {
		flusher.append("managed runtime cache appears stale; repairing exact execution cache\n")
		flusher.flush()
		var repairErr error
		if invalidator, ok := s.updater.(ExactRuntimeCacheInvalidator); ok {
			repairErr = invalidator.InvalidateExecutionCacheVersion(ctx, spec.Package, target)
		} else {
			repairErr = s.updater.InvalidateExecutionCache(ctx, spec.Package)
		}
		if repairErr != nil {
			s.finishFailed(job, ctx, fmt.Errorf("repair runtime execution cache: %w", repairErr), ref)
			return
		}
		flusher.append("retrying managed runtime update\n")
		flusher.flush()
		err = s.updater.RunUpdate(ctx, prepareCommand, flusher.append)
		flusher.flush()
		if err != nil {
			s.finishFailed(job, ctx, fmt.Errorf("update runtime after cache repair: %w", err), ref)
			return
		}
	}

	s.setStatus(job, dto.AgentUpdateJobStatusRefreshing)
	caps, probeErr := candidate.Probe(ctx, job.AgentName, spec.ACPCommand(target))
	if probeErr != nil {
		s.finishFailed(job, ctx, fmt.Errorf("probe runtime candidate: %w", probeErr), ref)
		return
	}
	if caps.Status != hostutility.StatusOK {
		s.finishFailed(job, ctx, errors.New(capabilityRefreshError(caps)), ref)
		return
	}
	s.mu.Lock()
	job.CurrentVersion = caps.AgentVersion
	s.mu.Unlock()
	if job.UseDefault {
		if err := s.selectionStore.Delete(ctx, job.AgentName, spec.Package); err != nil {
			s.finishFailed(job, ctx, fmt.Errorf("clear active runtime version: %w", err), ref)
			return
		}
	} else if err := s.selectionStore.Save(ctx, job.AgentName, spec.Package, target); err != nil {
		s.finishFailed(job, ctx, fmt.Errorf("persist active runtime version: %w", err), ref)
		return
	}
	s.mu.Lock()
	if job.UseDefault {
		job.ActiveVersion = ""
		job.EffectiveVersion = job.DefaultVersion
	} else {
		job.ActiveVersion = target
		job.EffectiveVersion = target
	}
	s.mu.Unlock()
	candidate.PublishCapabilities(job.AgentName, caps)
	s.finishActivated(job, target, ref)
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

func (s *AgentUpdateJobStore) finishAlreadyUpToDate(job *AgentUpdateJob, ref MaintenanceJobRef) {
	s.mu.Lock()
	job.Status = dto.AgentUpdateJobStatusSucceeded
	_, _ = job.Output.Write([]byte("Runtime already up to date.\n"))
	s.maintenance.release(job.AgentName, ref)
	s.finishLocked(job)
	snapshot := job.snapshot()
	s.mu.Unlock()
	s.broadcast(ws.ActionAgentUpdateFinished, snapshot)
	s.scheduleEviction(job.ID)
}

func (s *AgentUpdateJobStore) finishActivated(
	job *AgentUpdateJob,
	target string,
	ref MaintenanceJobRef,
) {
	s.mu.Lock()
	job.Status = dto.AgentUpdateJobStatusSucceeded
	if !job.UseDefault {
		job.ActiveVersion = target
		job.EffectiveVersion = target
	} else {
		job.ActiveVersion = ""
		job.EffectiveVersion = job.DefaultVersion
	}
	s.maintenance.release(job.AgentName, ref)
	s.finishLocked(job)
	snapshot := job.snapshot()
	statusInvalidator := s.onStatusInvalidated
	packageName := job.Package
	s.mu.Unlock()
	if statusInvalidator != nil && packageName != "" {
		statusInvalidator(packageName)
	}
	if s.onRefresh != nil {
		s.onRefresh()
	}
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
		job.CurrentVersion = caps.AgentVersion
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
	statusInvalidator := s.onStatusInvalidated
	packageName := job.Package
	s.mu.Unlock()
	if refreshed && statusInvalidator != nil && packageName != "" {
		statusInvalidator(packageName)
	}
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
		JobID:            j.ID,
		AgentName:        j.AgentName,
		Status:           j.Status,
		Operation:        string(j.Operation),
		CurrentVersion:   j.CurrentVersion,
		DefaultVersion:   j.DefaultVersion,
		ActiveVersion:    j.ActiveVersion,
		EffectiveVersion: j.EffectiveVersion,
		TargetVersion:    j.TargetVersion,
		Output:           j.Output.String(),
		Error:            j.Error,
		RefreshError:     j.RefreshError,
		StartedAt:        j.StartedAt,
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
