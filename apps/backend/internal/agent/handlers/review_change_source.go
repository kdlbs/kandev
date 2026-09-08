package handlers

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

// ReviewChangeSource reads a session's changed-file payloads for a native
// code-review pass.
//
// It lives here rather than in internal/review because this package already owns
// agentctl git access; internal/review stays free of the runtime-tier agentctl
// import (see apps/backend/AGENTS.md) and consumes this through a structural
// interface, so the two packages never import each other.
type ReviewChangeSource struct {
	executions    ExecutionLookup
	sessionReader SessionReader
}

// NewReviewChangeSource builds a change source over the execution lookup.
func NewReviewChangeSource(executions ExecutionLookup, sessionReader SessionReader) *ReviewChangeSource {
	return &ReviewChangeSource{executions: executions, sessionReader: sessionReader}
}

// UncommittedFiles returns the working-tree per-file payloads.
//
// Uses GetOrEnsureExecution because reviewing is a workspace-oriented read that
// must survive a backend restart, not an operation needing a running agent turn
// (see the "Execution access" convention in AGENTS.md).
func (s *ReviewChangeSource) UncommittedFiles(ctx context.Context, sessionID string) (map[string]any, error) {
	execution, err := s.execution(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil, fmt.Errorf("session %s workspace is not ready", sessionID)
	}
	status, err := client.GetGitStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("git status for session %s: %w", sessionID, err)
	}
	if status == nil {
		return nil, nil
	}
	return status.Files, nil
}

// CommittedFiles returns the cumulative committed-on-branch per-file payloads.
//
// A task with no base commit legitimately has nothing committed yet, which is
// reported as an empty map rather than an error so uncommitted work alone is
// still reviewable.
func (s *ReviewChangeSource) CommittedFiles(ctx context.Context, sessionID string) (map[string]any, error) {
	execution, err := s.execution(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil, fmt.Errorf("session %s workspace is not ready", sessionID)
	}

	baseCommit, targetBranch := "", ""
	if s.sessionReader != nil {
		baseCommit = s.sessionReader.GetSessionBaseCommit(ctx, sessionID)
		targetBranch = s.sessionReader.GetSessionBaseBranch(ctx, sessionID)
	}
	if baseCommit == "" {
		status, statusErr := client.GetGitStatus(ctx)
		if statusErr != nil || status == nil || status.BaseCommit == "" {
			return nil, nil
		}
		baseCommit = status.BaseCommit
	}

	result, err := client.GetCumulativeDiff(ctx, baseCommit, targetBranch)
	if err != nil {
		return nil, fmt.Errorf("cumulative diff for session %s: %w", sessionID, err)
	}
	if result == nil {
		return nil, nil
	}
	return result.Files, nil
}

// execution resolves the session's execution and guarantees a usable agentctl
// client, so callers can dereference it without another nil check.
func (s *ReviewChangeSource) execution(ctx context.Context, sessionID string) (*lifecycle.AgentExecution, error) {
	if s.executions == nil {
		return nil, fmt.Errorf("no execution lookup configured")
	}
	execution, err := s.executions.GetOrEnsureExecution(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("execution for session %s: %w", sessionID, err)
	}
	if execution == nil {
		return nil, fmt.Errorf("session %s workspace is not ready", sessionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	releaseClient()
	if client == nil {
		return nil, fmt.Errorf("session %s workspace is not ready", sessionID)
	}
	return execution, nil
}
