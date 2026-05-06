package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/office/configloader"
)

// ParseError describes a single file that failed to parse during a FS scan.
type ParseError struct {
	WorkspaceID string `json:"workspace_id"`
	FilePath    string `json:"file_path"`
	Error       string `json:"error"`
}

// SyncDiff is a bidirectional preview returned by sync endpoints.
type SyncDiff struct {
	Direction string         `json:"direction"`
	Preview   *ImportPreview `json:"preview"`
	Errors    []ParseError   `json:"errors,omitempty"`
}

// ScanFilesystem reads the on-disk workspace config into a ConfigBundle and
// returns any per-file parse errors. Returns nil bundle if no FS config exists.
func (s *Service) ScanFilesystem(_ context.Context, _ string) (*ConfigBundle, []ParseError, error) {
	if s.cfgLoader == nil {
		return nil, nil, fmt.Errorf("config loader not initialized")
	}
	if err := s.cfgLoader.Load(); err != nil {
		return nil, nil, fmt.Errorf("scan workspaces: %w", err)
	}
	bundle := &ConfigBundle{Settings: SettingsConfig{Name: defaultWorkspaceName}}
	for _, a := range s.cfgLoader.GetAgents(defaultWorkspaceName) {
		bundle.Agents = append(bundle.Agents, AgentConfig{
			Name: a.Name, Role: string(a.Role), Icon: a.Icon,
			BudgetMonthlyCents: a.BudgetMonthlyCents, MaxConcurrentSessions: a.MaxConcurrentSessions,
			DesiredSkills: a.DesiredSkills, ExecutorPreference: a.ExecutorPreference,
		})
	}
	for _, sk := range s.cfgLoader.GetSkills(defaultWorkspaceName) {
		bundle.Skills = append(bundle.Skills, SkillConfig{
			Name: sk.Name, Slug: sk.Slug, Description: sk.Description,
			SourceType: sk.SourceType, Content: sk.Content,
		})
	}
	for _, r := range s.cfgLoader.GetRoutines(defaultWorkspaceName) {
		bundle.Routines = append(bundle.Routines, RoutineConfig{
			Name: r.Name, Description: r.Description, TaskTemplate: r.TaskTemplate,
			ConcurrencyPolicy: r.ConcurrencyPolicy,
		})
	}
	for _, p := range s.cfgLoader.GetProjects(defaultWorkspaceName) {
		bundle.Projects = append(bundle.Projects, ProjectConfig{
			Name: p.Name, Description: p.Description, Status: string(p.Status),
			Color: p.Color, BudgetCents: p.BudgetCents,
			Repositories: p.Repositories, ExecutorConfig: p.ExecutorConfig,
		})
	}
	return bundle, parseErrorsFromLoader(s.cfgLoader), nil
}

func parseErrorsFromLoader(cl *configloader.ConfigLoader) []ParseError {
	src := cl.GetErrors()
	if len(src) == 0 {
		return nil
	}
	out := make([]ParseError, len(src))
	for i, e := range src {
		out[i] = ParseError{WorkspaceID: e.WorkspaceID, FilePath: e.FilePath, Error: e.Error}
	}
	return out
}

// IncomingDiff returns the changes that applying the on-disk config to the DB
// would produce (FS → DB), including deletions of DB rows not present on disk.
func (s *Service) IncomingDiff(ctx context.Context, workspaceID string) (*SyncDiff, error) {
	bundle, parseErrs, err := s.ScanFilesystem(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	preview, err := s.PreviewImport(ctx, workspaceID, bundle)
	if err != nil {
		return nil, err
	}
	if err := s.appendDeletions(ctx, workspaceID, bundle, preview); err != nil {
		return nil, err
	}
	return &SyncDiff{Direction: "incoming", Preview: preview, Errors: parseErrs}, nil
}

// OutgoingDiff returns the changes that exporting the DB to the on-disk config
// would produce (DB → FS), including deletions of FS files not present in DB.
func (s *Service) OutgoingDiff(ctx context.Context, workspaceID string) (*SyncDiff, error) {
	dbBundle, err := s.ExportBundle(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	fsBundle, parseErrs, err := s.ScanFilesystem(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	preview := diffBundles(dbBundle, fsBundle)
	return &SyncDiff{Direction: "outgoing", Preview: preview, Errors: parseErrs}, nil
}

// ApplyIncoming reads the on-disk config and writes it to the DB. Rows in the
// DB but missing from disk are deleted.
func (s *Service) ApplyIncoming(ctx context.Context, workspaceID string) (*ImportResult, error) {
	bundle, _, err := s.ScanFilesystem(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	result, err := s.ApplyImport(ctx, workspaceID, bundle)
	if err != nil {
		return nil, err
	}
	if delErr := s.deleteRowsMissingFromBundle(ctx, workspaceID, bundle); delErr != nil {
		return nil, delErr
	}
	return result, nil
}

// ApplyOutgoing writes the current DB state to the on-disk config.
func (s *Service) ApplyOutgoing(ctx context.Context, workspaceID string) error {
	if s.cfgWriter == nil {
		return fmt.Errorf("config writer not initialized")
	}
	bundle, err := s.ExportBundle(ctx, workspaceID)
	if err != nil {
		return err
	}
	return writeBundleToFS(s.cfgWriter, bundle)
}
