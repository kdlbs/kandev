package codexappserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

// Background terminal capabilities for Codex app-server.
func codexBackgroundTerminalCapabilities() streams.WorkloadCapabilities {
	return streams.WorkloadCapabilities{
		Discovery:         "snapshot",
		Output:            "snapshot",
		Transcript:        false,
		Parentage:         false,
		ReasoningSummary:  false,
		AttributableUsage: false,
		Actions: map[streams.WorkloadActionKind]streams.ActionCapability{
			streams.WorkloadActionStop: {
				Supported: true,
				Available: true,
			},
			streams.WorkloadActionInterrupt: {
				Supported: false,
				Available: false,
				Reason:    string(streams.ActionReasonUnsupported),
			},
			streams.WorkloadActionWriteInput: {
				Supported: false,
				Available: false,
				Reason:    string(streams.ActionReasonUnsupported),
			},
			streams.WorkloadActionCloseInput: {
				Supported: false,
				Available: false,
				Reason:    string(streams.ActionReasonUnsupported),
			},
		},
	}
}

// Subagent capabilities for Codex app-server.
func codexSubagentCapabilities() streams.WorkloadCapabilities {
	return streams.WorkloadCapabilities{
		Discovery:         "events_only",
		Output:            "none",
		Transcript:        false,
		Parentage:         true,
		ReasoningSummary:  false,
		AttributableUsage: true,
		Actions: map[streams.WorkloadActionKind]streams.ActionCapability{
			streams.WorkloadActionStop: {
				Supported: false,
				Available: false,
				Reason:    string(streams.ActionReasonUnsupported),
			},
			streams.WorkloadActionInterrupt: {
				Supported: true,
				Available: true,
			},
			streams.WorkloadActionWriteInput: {
				Supported: false,
				Available: false,
				Reason:    string(streams.ActionReasonUnsupported),
			},
			streams.WorkloadActionCloseInput: {
				Supported: false,
				Available: false,
				Reason:    string(streams.ActionReasonUnsupported),
			},
		},
	}
}

// ListBackgroundWorkloads returns active and recent background workloads.
func childStateFromStatus(status string) streams.RunState {
	if !isTerminalChildStatus(status) {
		return streams.RunStateRunning
	}
	switch status {
	case childStatusFailed, childStatusError, childStatusErrored:
		return streams.RunStateFailed
	case childStatusInterrupted, childStatusCancelled, childStatusCanceled:
		return streams.RunStateInterrupted
	default:
		return streams.RunStateCompleted
	}
}

// ListBackgroundWorkloads returns active and recent background workloads.
func (a *Adapter) ListBackgroundWorkloads(ctx context.Context, sessionID string) ([]streams.WorkloadRunObservation, error) {
	a.mu.RLock()
	threadID := a.threadID
	if a.closed || a.client == nil || threadID == "" || (sessionID != "" && sessionID != threadID) {
		a.mu.RUnlock()
		return []streams.WorkloadRunObservation{}, nil
	}

	workloads := make([]streams.WorkloadRunObservation, 0, len(a.backgrounds)+len(a.children))
	now := time.Now().UTC()

	// Background terminals
	for _, term := range a.backgrounds {
		workloads = append(workloads, streams.WorkloadRunObservation{
			SessionID:    threadID,
			WorkID:       term.ItemID,
			RunID:        term.ProcessID,
			Kind:         streams.WorkloadKindShell,
			Title:        term.Command,
			State:        streams.RunStateRunning,
			SourceCallID: term.ItemID,
			Capabilities: codexBackgroundTerminalCapabilities(),
			StartedAt:    &now,
		})
	}

	// Subagents
	for childThreadID, binding := range a.children {
		status := a.childStatuses[binding.toolCallID]
		if status == "" {
			status = a.childStatuses[childThreadID]
		}
		workloads = append(workloads, streams.WorkloadRunObservation{
			SessionID:    binding.parentThreadID,
			WorkID:       childThreadID,
			Kind:         streams.WorkloadKindSubagent,
			Title:        binding.description,
			State:        childStateFromStatus(status),
			ParentWorkID: binding.toolCallID,
			SourceCallID: binding.toolCallID,
			Capabilities: codexSubagentCapabilities(),
			StartedAt:    &now,
		})
	}
	a.mu.RUnlock()

	return workloads, nil
}

// GetBackgroundWorkload returns a specific workload by ID.
func (a *Adapter) GetBackgroundWorkload(ctx context.Context, sessionID, workID string) (*streams.WorkloadRunObservation, error) {
	workloads, err := a.ListBackgroundWorkloads(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	for _, w := range workloads {
		if w.WorkID == workID || w.RunID == workID || w.SourceCallID == workID {
			return &w, nil
		}
	}
	return nil, nil
}

func (a *Adapter) executeStopAction(ctx context.Context, threadID string, req streams.BackgroundWorkActionRequest) (streams.BackgroundWorkActionResponse, error) {
	a.mu.RLock()
	client := a.client
	var targetProcessID string
	for _, term := range a.backgrounds {
		if term.ItemID == req.WorkID || term.ProcessID == req.WorkID || (req.RunID != "" && term.ProcessID == req.RunID) {
			targetProcessID = term.ProcessID
			break
		}
	}
	a.mu.RUnlock()

	if targetProcessID == "" {
		return streams.BackgroundWorkActionResponse{
			Success: false,
			WorkID:  req.WorkID,
			RunID:   req.RunID,
			Action:  req.Action,
			Error:   "background terminal not found or already completed",
		}, nil
	}

	terminated, err := client.TerminateBackgroundTerminal(ctx, threadID, targetProcessID)
	if err != nil {
		return streams.BackgroundWorkActionResponse{
			Success: false,
			WorkID:  req.WorkID,
			RunID:   targetProcessID,
			Action:  req.Action,
			Error:   err.Error(),
		}, nil
	}
	return streams.BackgroundWorkActionResponse{
		Success: terminated,
		WorkID:  req.WorkID,
		RunID:   targetProcessID,
		Action:  req.Action,
	}, nil
}

func (a *Adapter) executeInterruptAction(ctx context.Context, req streams.BackgroundWorkActionRequest) (streams.BackgroundWorkActionResponse, error) {
	a.mu.RLock()
	client := a.client
	var targetThreadID string
	var activeTurnID string
	for childID, binding := range a.children {
		if childID == req.WorkID || binding.toolCallID == req.WorkID {
			targetThreadID = childID
			activeTurnID = binding.activeTurnID
			break
		}
	}
	a.mu.RUnlock()

	if targetThreadID == "" {
		return streams.BackgroundWorkActionResponse{
			Success: false,
			WorkID:  req.WorkID,
			RunID:   req.RunID,
			Action:  req.Action,
			Error:   "child subagent thread not found or already completed",
		}, nil
	}

	if err := client.InterruptTurn(ctx, targetThreadID, activeTurnID); err != nil {
		return streams.BackgroundWorkActionResponse{
			Success: false,
			WorkID:  req.WorkID,
			Action:  req.Action,
			Error:   err.Error(),
		}, nil
	}
	return streams.BackgroundWorkActionResponse{
		Success: true,
		WorkID:  req.WorkID,
		Action:  req.Action,
	}, nil
}

// BackgroundWorkSnapshot returns the current snapshot of all background workloads.
func (a *Adapter) BackgroundWorkSnapshot(ctx context.Context) (*streams.BackgroundWorkSnapshot, error) {
	workloads, err := a.ListBackgroundWorkloads(ctx, "")
	if err != nil {
		return nil, err
	}
	return &streams.BackgroundWorkSnapshot{
		Workloads:  workloads,
		CapturedAt: time.Now().UTC(),
	}, nil
}

// PerformBackgroundWorkAction executes an action on a background workload.
func (a *Adapter) PerformBackgroundWorkAction(ctx context.Context, req streams.BackgroundWorkActionRequest) (*streams.BackgroundWorkActionResponse, error) {
	resp, err := a.ExecuteBackgroundAction(ctx, req, "")
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// ExecuteBackgroundAction performs a targeted action on a background workload.
func (a *Adapter) ExecuteBackgroundAction(
	ctx context.Context,
	req streams.BackgroundWorkActionRequest,
	sessionID string,
) (streams.BackgroundWorkActionResponse, error) {
	switch req.Action {
	case streams.WorkloadActionWriteInput, streams.WorkloadActionCloseInput:
		return streams.BackgroundWorkActionResponse{
			Success: false,
			WorkID:  req.WorkID,
			RunID:   req.RunID,
			Action:  req.Action,
			Error:   "input writing is unsupported by Codex app-server",
		}, nil
	case streams.WorkloadActionStop, streams.WorkloadActionInterrupt:
		// Supported
	default:
		return streams.BackgroundWorkActionResponse{
			Success: false,
			WorkID:  req.WorkID,
			RunID:   req.RunID,
			Action:  req.Action,
			Error:   fmt.Sprintf("unsupported action: %s", req.Action),
		}, nil
	}

	a.mu.RLock()
	client := a.client
	threadID := a.threadID
	closed := a.closed
	a.mu.RUnlock()

	if closed || client == nil || threadID == "" {
		return streams.BackgroundWorkActionResponse{
			Success: false,
			WorkID:  req.WorkID,
			RunID:   req.RunID,
			Action:  req.Action,
			Error:   "adapter not connected or thread not active",
		}, nil
	}

	if req.Action == streams.WorkloadActionStop {
		return a.executeStopAction(ctx, threadID, req)
	}
	return a.executeInterruptAction(ctx, req)
}

// FetchAllBackgroundTerminals fetches all pages of background terminals from the client.
func (a *Adapter) FetchAllBackgroundTerminals(ctx context.Context, threadID string) ([]protocol.BackgroundTerminal, error) {
	client := a.getClient()
	if client == nil || threadID == "" {
		return nil, errors.New("client not available")
	}

	var all []protocol.BackgroundTerminal
	var cursor *string

	for {
		params := protocol.BackgroundTerminalsListParams{
			ThreadID: threadID,
			Cursor:   cursor,
		}
		resp, err := client.ListBackgroundTerminals(ctx, params)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Data...)
		if resp.NextCursor == nil || *resp.NextCursor == "" {
			break
		}
		cursor = resp.NextCursor
	}
	return all, nil
}
