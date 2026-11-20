package promptsync

import (
	"context"
	"sort"

	promptmodels "github.com/kandev/kandev/internal/prompts/models"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/reposync"
)

// Conflict is a repository prompt file whose name a manual, built-in, or
// still-conflicted override prompt already uses.
type Conflict struct {
	Name              string `json:"name"`
	Path              string `json:"path"`
	LocalContent      string `json:"local_content"`
	RepositoryContent string `json:"repository_content"`
}

// CheckResult lists the conflicts a source would cause, plus file warnings.
type CheckResult struct {
	Conflicts []Conflict `json:"conflicts"`
	Warnings  []string   `json:"warnings"`
}

// SaveOutcome is a saved configuration and the sync that followed. SyncErr
// is a failed sync; the configuration is saved either way.
type SaveOutcome struct {
	Config  *Config
	Result  *SyncResult
	SyncErr error
}

// Check reads a source and reports its conflicts without storing or changing
// anything. A nil request checks the stored configuration.
func (s *Service) Check(ctx context.Context, req *SetConfigRequest) (*CheckResult, error) {
	cfg, err := s.checkTarget(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.checkWorkspace(ctx, cfg.WorkspaceID); err != nil {
		return nil, err
	}
	if cfg.Provider != reposync.ProviderGitLab && s.github != nil {
		if ctx, err = s.github.BindWorkspaceGitHubHost(ctx, cfg.WorkspaceID, cfg.GitHubHost); err != nil {
			return nil, err
		}
	}
	files, err := s.fetch(ctx, cfg)
	if err != nil {
		return nil, err
	}
	desired, _, warnings := parsePromptFiles(files)
	prompts, err := s.prompts.ListPrompts(ctx)
	if err != nil {
		return nil, err
	}
	return &CheckResult{Conflicts: findConflicts(desired, prompts), Warnings: warnings}, nil
}

func (s *Service) checkTarget(ctx context.Context, req *SetConfigRequest) (*Config, error) {
	if req == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.lockedAuthorizedConfig(ctx)
	}
	if err := req.Normalize(); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, req.WorkspaceID); err != nil {
		return nil, err
	}
	return configFromRequest(req), nil
}

// findConflicts pairs each desired file with an existing prompt of the same
// name that sync does not own: a manual or built-in prompt, or an override
// whose conflict the user has not resolved.
func findConflicts(desired []promptservice.SyncedPromptFile, prompts []*promptmodels.Prompt) []Conflict {
	byName := make(map[string]*promptmodels.Prompt, len(prompts))
	for _, prompt := range prompts {
		byName[prompt.Name] = prompt
	}
	conflicts := []Conflict{}
	for _, file := range desired {
		prompt := byName[file.Name]
		if prompt == nil || prompt.IsSynced() || (prompt.IsOverride() && !prompt.Conflict) {
			continue
		}
		conflicts = append(conflicts, Conflict{
			Name: file.Name, Path: file.Path, LocalContent: prompt.Content, RepositoryContent: file.Content,
		})
	}
	return conflicts
}

// SaveAndSync stores the configuration and runs one sync with the user's
// conflict choices, under one hold of the service lock.
func (s *Service) SaveAndSync(ctx context.Context, req *SetConfigRequest) (*SaveOutcome, error) {
	if err := req.Normalize(); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, req.WorkspaceID); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.saveLocked(ctx, req)
	if err != nil {
		return nil, err
	}
	result, syncErr := s.syncLocked(ctx, syncManual, cfg, req.Resolutions)
	if refreshed, getErr := s.GetConfig(ctx); getErr == nil && refreshed != nil {
		cfg = refreshed
	}
	return &SaveOutcome{Config: cfg, Result: result, SyncErr: syncErr}, nil
}

// KeptVersions lists synced prompts that keep a local version beside the
// repository version, sorted by name, so removing the configuration can offer
// to restore them.
func (s *Service) KeptVersions(ctx context.Context) ([]Conflict, error) {
	prompts, err := s.prompts.ListPrompts(ctx)
	if err != nil {
		return nil, err
	}
	kept := []Conflict{}
	for _, prompt := range prompts {
		if !prompt.IsSynced() || prompt.LocalContent == "" {
			continue
		}
		kept = append(kept, Conflict{
			Name: prompt.Name, Path: prompt.SourcePath, LocalContent: prompt.LocalContent, RepositoryContent: prompt.Content,
		})
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Name < kept[j].Name })
	return kept, nil
}
