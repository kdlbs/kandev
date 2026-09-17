package orchestrator

import (
	"context"
	"sync"

	"go.uber.org/zap"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// pendingStepPrompt is the composed content of a workflow step-entry prompt
// that armed a CREATED-session auto-start launch. startAgentProcessAsync
// returns before the agent process is known to have started, so the launch
// call site never sees the eventual outcome; this handle lets the
// asynchronous success/failure callbacks decide what happens to the prompt.
type pendingStepPrompt struct {
	taskID              string
	prompt              string
	planMode            bool
	attachments         []v1.MessageAttachment
	origin              workflowMessageOrigin
	userMessageRecorded bool
	references          []v1.EntityReference
	handoffText         string
}

// pendingStepPromptRegistry holds at most one armed handle per session.
// take is single-use: it removes the entry it returns, so a second callback
// for the same launch finds nothing to act on.
type pendingStepPromptRegistry struct {
	mu      sync.Mutex
	entries map[string]pendingStepPrompt
}

func newPendingStepPromptRegistry() *pendingStepPromptRegistry {
	return &pendingStepPromptRegistry{entries: make(map[string]pendingStepPrompt)}
}

// arm records the prompt a CREATED-session launch is about to carry through
// startAgentProcessAsync, replacing any handle already armed for the session.
func (r *pendingStepPromptRegistry) arm(sessionID string, p pendingStepPrompt) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[sessionID] = p
}

// discard removes an armed handle without acting on it, for outcomes another
// path already owns (a synchronous launch failure, or a successful start
// whose prompt already reached the agent as the execution description).
func (r *pendingStepPromptRegistry) discard(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, sessionID)
}

// take removes and returns the armed handle for sessionID, if any.
func (r *pendingStepPromptRegistry) take(sessionID string) (pendingStepPrompt, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.entries[sessionID]
	if ok {
		delete(r.entries, sessionID)
	}
	return p, ok
}

// pendingStepPromptStore lazily initializes the registry, mirroring
// resumeAttemptStore's pattern.
func (s *Service) pendingStepPromptStore() *pendingStepPromptRegistry {
	s.pendingStepPromptsMu.Lock()
	defer s.pendingStepPromptsMu.Unlock()
	if s.pendingStepPrompts == nil {
		s.pendingStepPrompts = newPendingStepPromptRegistry()
	}
	return s.pendingStepPrompts
}

// armPendingStepPrompt records prompt content that a CREATED-session
// auto-start launch is about to carry through an asynchronous agent-process
// start, so a start failure that surfaces after the launch call returns can
// still recover and re-queue it.
func (s *Service) armPendingStepPrompt(sessionID string, p pendingStepPrompt) {
	s.pendingStepPromptStore().arm(sessionID, p)
}

// discardPendingStepPrompt drops an armed handle whose outcome is already
// owned by another path (synchronous failure, or a successful start).
func (s *Service) discardPendingStepPrompt(sessionID string) {
	s.pendingStepPromptStore().discard(sessionID)
}

// recoverPendingStepPromptOnAsyncStartFailure takes the pending step prompt
// armed for sessionID, if any, and re-queues it without scheduling an
// auto-resume: the caller is already mid-projection for a start failure (or
// its recoverable-failure retry), and scheduling a resume here would race
// that projection or loop against a start that keeps failing. Delivery
// happens on the session's next promptable transition through the existing
// boot-ready drain, the same contract the synchronous failure path
// (handleCreatedAutoStartLaunchFailure) already relies on. A superseded
// execution, a stale resume attempt, or an in-flight cancellation must not
// reach this call — those callers return before taking the handle, leaving
// it armed for the execution that actually owns the session.
func (s *Service) recoverPendingStepPromptOnAsyncStartFailure(ctx context.Context, sessionID string) {
	pending, ok := s.pendingStepPromptStore().take(sessionID)
	if !ok {
		return
	}
	if err := s.queueAsyncStartFailurePrompt(
		ctx, pending.taskID, sessionID, pending.prompt, pending.planMode,
		pending.attachments, pending.origin, pending.userMessageRecorded,
		pending.references, pending.handoffText,
	); err != nil {
		s.logger.Warn("failed to queue step prompt after asynchronous start failure",
			zap.String("task_id", pending.taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}
