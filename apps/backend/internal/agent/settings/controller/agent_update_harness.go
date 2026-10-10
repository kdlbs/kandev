package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (s *AgentUpdateJobStore) EnqueueHarness(
	agentName string, spec agents.HarnessUpdateSpec, probe agents.Command,
) (*AgentUpdateJob, error) {
	s.mu.Lock()
	if existing := s.activeByAgt[agentName]; existing != nil {
		s.mu.Unlock()
		return existing, nil
	}
	job := &AgentUpdateJob{
		ID: uuid.NewString(), AgentName: agentName, Package: spec.Package,
		UpdateMode: dto.AgentUpdateModeSelfUpdate, Status: dto.AgentUpdateJobStatusQueued,
		Output: newRingBuffer(jobOutputRingSize), StartedAt: time.Now().UTC(),
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
	go s.runHarness(job, spec, probe, ref)
	return job, nil
}

func (s *AgentUpdateJobStore) runHarness(
	job *AgentUpdateJob, spec agents.HarnessUpdateSpec, probe agents.Command, ref MaintenanceJobRef,
) {
	s.semaphore <- struct{}{}
	defer func() { <-s.semaphore }()
	ctx, cancel := context.WithTimeout(context.Background(), jobHardTimeout)
	defer cancel()
	s.setStatus(job, dto.AgentUpdateJobStatusResolving)
	current := ""
	if caps, found := s.updater.CurrentCapabilities(job.AgentName); found {
		current = caps.AgentVersion
	}
	s.mu.Lock()
	job.CurrentVersion = current
	job.EffectiveVersion = current
	job.Operation = managedruntime.Operation(harnessUpdateOperation(current))
	s.mu.Unlock()
	candidate, ok := s.updater.(RuntimeCandidateUpdater)
	if !ok {
		s.finishFailed(job, ctx, fmt.Errorf("probe unavailable for harness-owned update"), ref)
		return
	}
	s.setStatus(job, dto.AgentUpdateJobStatusUpdating)
	flusher := newUpdateOutputFlusher(s, job)
	err := s.updater.RunUpdate(ctx, spec.UpdateCommand, flusher.append)
	flusher.flush()
	if err != nil {
		s.finishFailed(job, ctx, fmt.Errorf("harness update: %w", err), ref)
		return
	}
	s.setStatus(job, dto.AgentUpdateJobStatusRefreshing)
	// Probe without publishing: an in-place update can fail ACP validation.
	caps, err := candidate.Probe(ctx, job.AgentName, probe)
	if err != nil {
		s.finishFailed(job, ctx, fmt.Errorf("probe updated harness: %w", err), ref)
		return
	}
	if caps.Status != hostutility.StatusOK {
		s.finishFailed(job, ctx, fmt.Errorf("probe updated harness: %s", capabilityRefreshError(caps)), ref)
		return
	}
	if caps.AgentVersion == current {
		s.finishFailed(job, ctx, fmt.Errorf("harness update did not change the ACP-reported version; check the updater output and installation channel"), ref)
		return
	}
	candidate.PublishCapabilities(job.AgentName, caps)
	s.mu.Lock()
	job.EffectiveVersion = caps.AgentVersion
	s.mu.Unlock()
	s.finishRefresh(job, ctx, caps, nil, ref)
}
