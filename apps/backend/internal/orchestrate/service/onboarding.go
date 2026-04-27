package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestrate/configloader"
	"github.com/kandev/kandev/internal/orchestrate/models"
	"go.uber.org/zap"
)

// OnboardingState holds the current onboarding state.
type OnboardingState struct {
	Completed   bool   `json:"completed"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	CEOAgentID  string `json:"ceoAgentId,omitempty"`
}

// OnboardingCompleteRequest holds the inputs for completing onboarding.
type OnboardingCompleteRequest struct {
	WorkspaceName      string
	TaskPrefix         string
	AgentName          string
	AgentProfileID     string
	ExecutorPreference string
	TaskTitle          string
	TaskDescription    string
}

// OnboardingCompleteResult holds the IDs of entities created during onboarding.
type OnboardingCompleteResult struct {
	WorkspaceID string
	AgentID     string
	ProjectID   string
	TaskID      string
}

// GetOnboardingState checks whether onboarding has been completed.
func (s *Service) GetOnboardingState(ctx context.Context) (*OnboardingState, error) {
	row, err := s.repo.GetFirstCompletedOnboarding(ctx)
	if err != nil {
		return nil, fmt.Errorf("check onboarding state: %w", err)
	}
	if row == nil {
		return &OnboardingState{Completed: false}, nil
	}
	return &OnboardingState{
		Completed:   true,
		WorkspaceID: row.WorkspaceID,
		CEOAgentID:  row.CEOAgentID,
	}, nil
}

// CompleteOnboarding creates workspace, CEO agent, project, optional task,
// and marks onboarding as finished.
func (s *Service) CompleteOnboarding(ctx context.Context, req OnboardingCompleteRequest) (*OnboardingCompleteResult, error) {
	result := &OnboardingCompleteResult{}

	// 1. Create workspace (filesystem + DB).
	if err := s.createOnboardingWorkspace(ctx, req.WorkspaceName, req.TaskPrefix); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	wsID, err := s.resolveKanbanWorkspaceID(ctx, req.WorkspaceName)
	if err != nil || wsID == "" {
		return nil, fmt.Errorf("workspace ID not found after creation")
	}
	result.WorkspaceID = wsID

	// 2. Create CEO agent instance.
	agentID, err := s.createOnboardingAgent(ctx, wsID, req)
	if err != nil {
		return nil, fmt.Errorf("create CEO agent: %w", err)
	}
	result.AgentID = agentID

	// 3. Create agent runtime row (idle).
	if rtErr := s.repo.UpsertAgentRuntime(ctx, agentID, string(models.AgentStatusIdle), ""); rtErr != nil {
		s.logger.Warn("create agent runtime failed", zap.Error(rtErr))
	}

	// 4. Create "Onboarding" project.
	projectID, err := s.createOnboardingProject(ctx, wsID, agentID)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	result.ProjectID = projectID

	// 5. Mark onboarding complete.
	if err := s.repo.MarkOnboardingComplete(ctx, wsID, agentID, ""); err != nil {
		return nil, fmt.Errorf("mark onboarding complete: %w", err)
	}

	s.logger.Info("onboarding completed",
		zap.String("workspace_id", wsID),
		zap.String("agent_id", agentID))
	return result, nil
}

// createOnboardingWorkspace writes the workspace config and DB row.
func (s *Service) createOnboardingWorkspace(ctx context.Context, name, taskPrefix string) error {
	slug := generateSlug(name)
	if s.cfgWriter != nil {
		settings := &configloader.WorkspaceSettings{
			Name:       name,
			Slug:       slug,
			TaskPrefix: taskPrefix,
		}
		if err := s.writeWorkspaceConfig(name, settings); err != nil {
			return err
		}
	}
	if s.workspaceCreator != nil {
		if err := s.workspaceCreator.CreateWorkspace(ctx, name, "Orchestrate workspace"); err != nil {
			s.logger.Warn("DB workspace creation failed",
				zap.String("name", name), zap.Error(err))
		}
	}
	return nil
}

// writeWorkspaceConfig marshals settings and writes them to the workspace directory.
func (s *Service) writeWorkspaceConfig(name string, settings *configloader.WorkspaceSettings) error {
	if !isValidPathComponent(name) {
		return fmt.Errorf("invalid workspace name")
	}
	data, err := configloader.MarshalSettings(*settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	wsDir := filepath.Join(s.cfgLoader.BasePath(), "workspaces", name)
	if mkErr := os.MkdirAll(wsDir, 0o755); mkErr != nil {
		return fmt.Errorf("create dir: %w", mkErr)
	}
	settingsPath := filepath.Join(wsDir, "kandev.yml")
	if writeErr := os.WriteFile(settingsPath, data, 0o644); writeErr != nil {
		return fmt.Errorf("write settings: %w", writeErr)
	}
	if reloadErr := s.cfgLoader.Reload(name); reloadErr != nil {
		return fmt.Errorf("reload config: %w", reloadErr)
	}
	return nil
}

var (
	slugNonAlphanumRe = regexp.MustCompile(`[^a-z0-9-]`)
	slugMultiDashRe   = regexp.MustCompile(`-+`)
)

// generateSlug creates a URL-safe slug from a workspace name.
func generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = slugNonAlphanumRe.ReplaceAllString(slug, "-")
	slug = slugMultiDashRe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "workspace"
	}
	if len(slug) > 50 {
		slug = slug[:50]
	}
	return slug
}

// resolveKanbanWorkspaceID looks up the kanban workspace UUID by name.
// Falls back to the workspace name when no WorkspaceCreator is configured.
func (s *Service) resolveKanbanWorkspaceID(ctx context.Context, name string) (string, error) {
	if s.workspaceCreator != nil {
		id, err := s.workspaceCreator.FindWorkspaceIDByName(ctx, name)
		if err != nil {
			return "", err
		}
		if id != "" {
			return id, nil
		}
	}
	// Fallback: use the workspace name as the ID (for tests without WorkspaceCreator).
	return name, nil
}

// createOnboardingAgent creates the CEO agent instance.
func (s *Service) createOnboardingAgent(ctx context.Context, wsID string, req OnboardingCompleteRequest) (string, error) {
	execPref := "{}"
	if req.ExecutorPreference != "" {
		execPref = fmt.Sprintf(`{"type":%q}`, req.ExecutorPreference)
	}
	agent := &models.AgentInstance{
		ID:                 uuid.New().String(),
		WorkspaceID:        wsID,
		Name:               req.AgentName,
		AgentProfileID:     req.AgentProfileID,
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		Permissions:        DefaultPermissions(models.AgentRoleCEO),
		ExecutorPreference: execPref,
	}
	if err := s.CreateAgentInstance(ctx, agent); err != nil {
		return "", err
	}
	return agent.ID, nil
}

// createOnboardingProject creates the default "Onboarding" project.
func (s *Service) createOnboardingProject(ctx context.Context, wsID, agentID string) (string, error) {
	project := &models.Project{
		ID:                  uuid.New().String(),
		WorkspaceID:         wsID,
		Name:                "Onboarding",
		Description:         "Default project created during workspace setup",
		Status:              models.ProjectStatusActive,
		LeadAgentInstanceID: agentID,
	}
	if err := s.CreateProject(ctx, project); err != nil {
		return "", err
	}
	return project.ID, nil
}
