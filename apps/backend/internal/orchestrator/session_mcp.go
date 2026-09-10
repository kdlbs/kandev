package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// MCPSelectionWriter stores validated MCP selections for a task session.
// The settings service performs workspace and owner validation before writing.
type MCPSelectionWriter interface {
	Replace(context.Context, mcpconfig.SelectionScope, string, string, []string) error
}

// SetMCPSelectionWriter wires the optional task-session selection store.
func (s *Service) SetMCPSelectionWriter(writer MCPSelectionWriter) {
	s.mcpSelectionWriter = writer
}

func (s *Service) applyMCPServerSelections(ctx context.Context, taskID, sessionID string, ids []string) error {
	if ids == nil {
		return nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to load task for MCP selections: %w", err)
	}
	if err := s.validateMCPSelectionSession(ctx, taskID, sessionID); err != nil {
		return err
	}
	return s.applyMCPServerSelectionsForWorkspace(ctx, task.WorkspaceID, sessionID, ids)
}

func (s *Service) applyMCPServerSelectionsForTask(ctx context.Context, task *v1.Task, sessionID string, ids []string) error {
	if task == nil {
		return nil
	}
	if err := s.validateMCPSelectionSession(ctx, task.ID, sessionID); err != nil {
		return err
	}
	return s.applyMCPServerSelectionsForWorkspace(ctx, task.WorkspaceID, sessionID, ids)
}

func (s *Service) validateMCPSelectionSession(ctx context.Context, taskID, sessionID string) error {
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to load session for MCP selections: %w", err)
	}
	if session == nil || session.TaskID != taskID {
		return fmt.Errorf("MCP selection session does not belong to task")
	}
	return nil
}

func (s *Service) applyMCPServerSelectionsForWorkspace(ctx context.Context, workspaceID, sessionID string, ids []string) error {
	if s.mcpSelectionWriter == nil || workspaceID == "" || sessionID == "" {
		return nil
	}
	if err := s.mcpSelectionWriter.Replace(
		ctx, mcpconfig.SelectionScopeTaskSession, workspaceID, sessionID, ids,
	); err != nil {
		return fmt.Errorf("failed to save MCP selections for session: %w", err)
	}
	return nil
}
