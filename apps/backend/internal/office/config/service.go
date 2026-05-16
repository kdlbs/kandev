package config

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"

	"gopkg.in/yaml.v3"
)

// defaultWorkspaceName is used for ConfigLoader lookups when we only have a
// DB workspace ID. Most single-user installs have one workspace named "default".
const defaultWorkspaceName = "default"

// ConfigService provides config export, import, and sync logic.
type ConfigService struct {
	repo      *sqlite.Repository
	cfgLoader *configloader.ConfigLoader
	cfgWriter *configloader.FileWriter
	logger    *logger.Logger
	activity  shared.ActivityLogger
}

// NewConfigService constructs a ConfigService.
func NewConfigService(
	repo *sqlite.Repository,
	cfgLoader *configloader.ConfigLoader,
	cfgWriter *configloader.FileWriter,
	log *logger.Logger,
	activity shared.ActivityLogger,
) *ConfigService {
	return &ConfigService{
		repo:      repo,
		cfgLoader: cfgLoader,
		cfgWriter: cfgWriter,
		logger:    log,
		activity:  activity,
	}
}

// ExportBundle exports the full workspace configuration as a ConfigBundle.
func (s *ConfigService) ExportBundle(ctx context.Context, workspaceID string) (*ConfigBundle, error) {
	bundle := &ConfigBundle{
		Settings: SettingsConfig{Name: workspaceID},
	}

	if err := s.exportAgents(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}
	if err := s.exportSkills(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}
	if err := s.exportRoutines(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}
	if err := s.exportProjects(ctx, workspaceID, bundle); err != nil {
		return nil, err
	}
	return bundle, nil
}

func (s *ConfigService) exportAgents(ctx context.Context, _ string, bundle *ConfigBundle) error {
	agents, err := s.repo.ListAgentInstances(ctx, "")
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}
	// Build ID->name map for reports_to resolution.
	nameByID := make(map[string]string, len(agents))
	for _, a := range agents {
		nameByID[a.ID] = a.Name
	}
	for _, a := range agents {
		cfg := AgentConfig{
			Name:                  a.Name,
			Role:                  string(a.Role),
			Icon:                  a.Icon,
			ReportsTo:             nameByID[a.ReportsTo],
			BudgetMonthlyCents:    a.BudgetMonthlyCents,
			MaxConcurrentSessions: a.MaxConcurrentSessions,
			DesiredSkills:         a.DesiredSkills,
			ExecutorPreference:    a.ExecutorPreference,
		}
		bundle.Agents = append(bundle.Agents, cfg)
	}
	return nil
}

func (s *ConfigService) exportSkills(ctx context.Context, _ string, bundle *ConfigBundle) error {
	skills, err := s.repo.ListSkills(ctx, "")
	if err != nil {
		return fmt.Errorf("list skills: %w", err)
	}
	for _, sk := range skills {
		cfg := SkillConfig{
			Name:        sk.Name,
			Slug:        sk.Slug,
			Description: sk.Description,
			SourceType:  sk.SourceType,
			Content:     sk.Content,
		}
		bundle.Skills = append(bundle.Skills, cfg)
	}
	return nil
}

func (s *ConfigService) exportRoutines(ctx context.Context, _ string, bundle *ConfigBundle) error {
	routines, err := s.repo.ListRoutines(ctx, "")
	if err != nil {
		return fmt.Errorf("list routines: %w", err)
	}
	agents, err := s.repo.ListAgentInstances(ctx, "")
	if err != nil {
		return fmt.Errorf("list agents for routine resolution: %w", err)
	}
	nameByID := make(map[string]string, len(agents))
	for _, a := range agents {
		nameByID[a.ID] = a.Name
	}
	for _, r := range routines {
		cfg := RoutineConfig{
			Name:              r.Name,
			Description:       r.Description,
			TaskTemplate:      r.TaskTemplate,
			AssigneeName:      nameByID[r.AssigneeAgentProfileID],
			ConcurrencyPolicy: r.ConcurrencyPolicy,
		}
		bundle.Routines = append(bundle.Routines, cfg)
	}
	return nil
}

func (s *ConfigService) exportProjects(ctx context.Context, _ string, bundle *ConfigBundle) error {
	projects, err := s.repo.ListProjects(ctx, "")
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	agents, err := s.repo.ListAgentInstances(ctx, "")
	if err != nil {
		return fmt.Errorf("list agents for project resolution: %w", err)
	}
	nameByID := make(map[string]string, len(agents))
	for _, a := range agents {
		nameByID[a.ID] = a.Name
	}
	for _, p := range projects {
		cfg := ProjectConfig{
			Name:           p.Name,
			Description:    p.Description,
			Status:         string(p.Status),
			Color:          p.Color,
			BudgetCents:    p.BudgetCents,
			Repositories:   p.Repositories,
			ExecutorConfig: p.ExecutorConfig,
			LeadAgentName:  nameByID[p.LeadAgentProfileID],
		}
		bundle.Projects = append(bundle.Projects, cfg)
	}
	return nil
}

// ExportZip exports the workspace configuration as a zip archive.
func (s *ConfigService) ExportZip(ctx context.Context, workspaceID string) (io.Reader, error) {
	bundle, err := s.ExportBundle(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return bundleToZip(bundle)
}

// bundleToZip converts a ConfigBundle to a zip archive.
func bundleToZip(bundle *ConfigBundle) (io.Reader, error) {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	if err := writeYAMLFile(w, ".kandev/kandev.yml", bundle.Settings); err != nil {
		return nil, err
	}
	for _, a := range bundle.Agents {
		if err := writeYAMLFile(w, ".kandev/agents/"+a.Name+".yml", a); err != nil {
			return nil, err
		}
	}
	for _, sk := range bundle.Skills {
		if err := writeYAMLFile(w, ".kandev/skills/"+sk.Slug+".yml", sk); err != nil {
			return nil, err
		}
	}
	for _, r := range bundle.Routines {
		if err := writeYAMLFile(w, ".kandev/routines/"+r.Name+".yml", r); err != nil {
			return nil, err
		}
	}
	for _, p := range bundle.Projects {
		if err := writeYAMLFile(w, ".kandev/projects/"+p.Name+".yml", p); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}
	return buf, nil
}

func writeYAMLFile(w *zip.Writer, name string, data interface{}) error {
	f, err := w.Create(name)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", name, err)
	}
	b, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	_, err = f.Write(b)
	return err
}
