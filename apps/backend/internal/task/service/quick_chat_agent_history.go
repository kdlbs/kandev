package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service/agenthistory"
)

// agentHistoryEnvKeys are the agent config-dir overrides read from the backend
// environment, which host-local agent processes inherit.
var agentHistoryEnvKeys = []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME"}

// agentHistoryTarget is the durable identity of one Quick Chat session's agent
// history, captured before the executor row that records it is deleted.
type agentHistoryTarget struct {
	SessionID   string            `json:"session_id"`
	AgentName   string            `json:"agent_name"`
	ResumeToken string            `json:"resume_token"`
	WorkDir     string            `json:"work_dir"`
	HomeDir     string            `json:"home_dir,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

// captureQuickChatAgentHistory returns the agent history targets of a Quick
// Chat's host-local sessions. Any other task yields none.
func (s *Service) captureQuickChatAgentHistory(
	ctx context.Context, task *models.Task, sessions []*models.TaskSession,
) []agentHistoryTarget {
	if !IsRestorableQuickChatTask(task) {
		return nil
	}
	home, _ := os.UserHomeDir()
	env := make(map[string]string, len(agentHistoryEnvKeys))
	for _, key := range agentHistoryEnvKeys {
		if value := os.Getenv(key); value != "" {
			env[key] = value
		}
	}
	var targets []agentHistoryTarget
	for _, session := range sessions {
		if session == nil || session.ID == "" || session.TaskID != task.ID {
			continue
		}
		records, err := retainedQuickChatAgentHistory(session)
		if err != nil {
			s.logger.Warn("quick-chat agent history: failed to read retained identities",
				zap.String("session_id", session.ID), zap.Error(err))
		}
		if current := s.currentQuickChatAgentHistory(ctx, task.ID, session); current != nil {
			records = append(records, *current)
		}
		targets = append(targets, quickChatHistoryTargets(session.ID, records, s.quickChatDir, home, env)...)
	}
	return targets
}

func quickChatHistoryTargets(
	sessionID string, records []models.QuickChatAgentHistory, quickChatDir, home string, env map[string]string,
) []agentHistoryTarget {
	var targets []agentHistoryTarget
	seen := make(map[models.QuickChatAgentHistory]struct{})
	for _, record := range records {
		if record.Runtime != agentruntime.RuntimeStandalone || record.ResumeToken == "" {
			continue
		}
		if _, duplicate := seen[record]; duplicate {
			continue
		}
		seen[record] = struct{}{}
		workDir := record.WorkDir
		if workDir == "" && quickChatDir != "" {
			workDir = filepath.Join(quickChatDir, sessionID)
		}
		targets = append(targets, agentHistoryTarget{
			SessionID: sessionID, AgentName: record.AgentName, ResumeToken: record.ResumeToken,
			WorkDir: workDir, HomeDir: home, Env: env,
		})
	}
	return targets
}

func retainedQuickChatAgentHistory(session *models.TaskSession) ([]models.QuickChatAgentHistory, error) {
	value, exists := session.Metadata[models.SessionMetaKeyQuickChatAgentHistory]
	if !exists {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var records []models.QuickChatAgentHistory
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Service) currentQuickChatAgentHistory(
	ctx context.Context, taskID string, session *models.TaskSession,
) *models.QuickChatAgentHistory {
	if s.executors == nil {
		return nil
	}
	running, err := s.executors.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil {
		if !errors.Is(err, models.ErrExecutorRunningNotFound) {
			s.logger.Warn("quick-chat agent history: failed to read executor runtime",
				zap.String("task_id", taskID), zap.String("session_id", session.ID), zap.Error(err))
		}
		return nil
	}
	if running == nil || running.TaskID != taskID || running.SessionID != session.ID ||
		running.Runtime != agentruntime.RuntimeStandalone || running.ResumeToken == "" {
		return nil
	}
	profileID := session.ExecutionProfileID
	if profileID == "" {
		profileID = session.AgentProfileID
	}
	if profileID != "" && running.ExecutionProfileID != "" && running.ExecutionProfileID != profileID {
		return nil
	}
	return &models.QuickChatAgentHistory{
		AgentName:   models.StringFromAny(session.AgentProfileSnapshot["agent_name"]),
		ResumeToken: running.ResumeToken, WorkDir: running.WorktreePath, Runtime: running.Runtime,
	}
}

// removeQuickChatAgentHistory removes captured agent history. Failures are
// logged and never change the deletion outcome.
func (s *Service) removeQuickChatAgentHistory(
	ctx context.Context, taskID string, targets []agentHistoryTarget, failedStops map[string]struct{},
) {
	for _, target := range targets {
		if _, preserve := failedStops[target.SessionID]; preserve {
			continue
		}
		err := agenthistory.Remove(ctx, agenthistory.Identity{
			AgentName:   target.AgentName,
			ResumeToken: target.ResumeToken,
			WorkDir:     target.WorkDir,
			Env:         target.Env,
			HomeDir:     target.HomeDir,
		})
		if err != nil {
			s.logger.Warn("failed to remove quick-chat agent history",
				zap.String("task_id", taskID),
				zap.String("session_id", target.SessionID),
				zap.String("agent", target.AgentName),
				zap.Error(err))
		}
	}
}
