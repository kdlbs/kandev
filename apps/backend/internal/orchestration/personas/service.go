// Package personas manages workspace coordinator identities on core execution profiles.
package personas

import (
	"context"
	"encoding/json"
	"fmt"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestration/repository/sqlite"
)

type ProfileStore interface {
	GetAgentProfile(context.Context, string) (*settings.AgentProfile, error)
	CreateAgentProfile(context.Context, *settings.AgentProfile) error
	UpdateAgentProfile(context.Context, *settings.AgentProfile) error
	DeleteAgentProfile(context.Context, string) error
}
type Service struct {
	Profiles  ProfileStore
	Repo      *sqlite.Repository
	Terminate func(context.Context, string) error
}

func (s *Service) GetAgentInstance(ctx context.Context, id string) (*settings.AgentProfile, error) {
	a, err := s.Profiles.GetAgentProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	roleID, err := s.Repo.OrchestratorRoleID(ctx, id)
	if err != nil {
		return nil, err
	}
	if roleID != "" {
		role, err := s.Repo.GetOrchestratorRole(ctx, roleID)
		if err != nil {
			return nil, err
		}
		a.Name = role.Name
	}
	return a, nil
}
func (s *Service) CreateAgentInstance(ctx context.Context, a *settings.AgentProfile) error {
	if a.WorkspaceID == "" || a.Role != settings.AgentRoleAssistant {
		return fmt.Errorf("workspace coordinator required")
	}
	a.Enabled = true
	return s.Profiles.CreateAgentProfile(ctx, a)
}
func (s *Service) UpdateAgentInstance(ctx context.Context, a *settings.AgentProfile) error {
	return s.Profiles.UpdateAgentProfile(ctx, a)
}
func (s *Service) DeleteAgentInstance(ctx context.Context, id string) error {
	if err := s.Profiles.DeleteAgentProfile(ctx, id); err != nil {
		return err
	}
	if s.Terminate != nil {
		return s.Terminate(ctx, id)
	}
	return nil
}
func (s *Service) UpdateAgentStatus(ctx context.Context, id string, status settings.AgentStatus, reason string) (*settings.AgentProfile, error) {
	a, err := s.Profiles.GetAgentProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	if status != settings.AgentStatusIdle && status != settings.AgentStatusPaused {
		return nil, fmt.Errorf("invalid coordinator status")
	}
	if status == settings.AgentStatusIdle && a.Status == settings.AgentStatusWorking {
		return nil, fmt.Errorf("coordinator is still working")
	}
	a.Status = status
	a.PauseReason = reason
	return a, s.Profiles.UpdateAgentProfile(ctx, a)
}
func (s *Service) GetInstruction(ctx context.Context, id, filename string) (*models.InstructionFile, error) {
	return s.Repo.GetInstruction(ctx, id, filename)
}
func (s *Service) UpsertInstruction(ctx context.Context, id, filename, content string, entry bool) error {
	return s.Repo.UpsertInstruction(ctx, id, filename, content, entry)
}
func (s *Service) ConfigurePinnedProfile(ctx context.Context, a *settings.AgentProfile, id string) error {
	source, err := s.Profiles.GetAgentProfile(ctx, id)
	if err != nil {
		return err
	}
	if source.Role != "" || source.Model == "" || !source.Enabled || (source.WorkspaceID != "" && source.WorkspaceID != a.WorkspaceID) {
		return fmt.Errorf("select an enabled execution profile in this workspace")
	}
	raw := map[string]json.RawMessage{}
	if a.Settings != "" {
		if err := json.Unmarshal([]byte(a.Settings), &raw); err != nil {
			return err
		}
	}
	if raw == nil {
		raw = map[string]json.RawMessage{}
	}
	raw["routing"], _ = json.Marshal(map[string]string{"execution_profile_id": id})
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	a.Settings = string(data)
	a.AgentID = source.AgentID
	return nil
}
func ExecutionProfileID(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	var data struct {
		Routing struct {
			ID string `json:"execution_profile_id"`
		} `json:"routing"`
	}
	err := json.Unmarshal([]byte(raw), &data)
	return data.Routing.ID, err
}
