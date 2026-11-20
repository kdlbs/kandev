package promptsync

import (
	"context"
	"sort"

	promptmodels "github.com/kandev/kandev/internal/prompts/models"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/reposync"
)

// Conflict is a prompt with a local and a repository version: a repository
// file whose name a manual, built-in, or still-conflicted override prompt
// already uses, or a synced prompt or override that keeps both versions.
// InUse, when set, names the version the prompt uses now.
type Conflict struct {
	Name              string                       `json:"name"`
	Path              string                       `json:"path"`
	LocalContent      string                       `json:"local_content"`
	RepositoryContent string                       `json:"repository_content"`
	InUse             promptservice.ConflictChoice `json:"in_use,omitempty"`
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
// whose conflict the user has not resolved. Identical text is no conflict.
func findConflicts(desired []promptservice.SyncedPromptFile, prompts []*promptmodels.Prompt) []Conflict {
	byName := make(map[string]*promptmodels.Prompt, len(prompts))
	for _, prompt := range prompts {
		byName[prompt.Name] = prompt
	}
	conflicts := []Conflict{}
	for _, file := range desired {
		prompt := byName[file.Name]
		if prompt == nil || prompt.IsSynced() || (prompt.IsOverride() && !prompt.Conflict) || prompt.Content == file.Content {
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

// RemovalReview is what removing the configuration asks about: prompts whose
// local and repository versions differ, and the names of synced prompts that
// only came from the repository. Both are sorted by name.
type RemovalReview struct {
	Prompts        []Conflict `json:"prompts"`
	RepositoryOnly []string   `json:"repository_only"`
}

// RemovalReview lists what removing the configuration asks about.
func (s *Service) RemovalReview(ctx context.Context) (*RemovalReview, error) {
	prompts, err := s.prompts.ListPrompts(ctx)
	if err != nil {
		return nil, err
	}
	review := &RemovalReview{Prompts: []Conflict{}, RepositoryOnly: []string{}}
	for _, prompt := range prompts {
		if item, ok := twoVersionPrompt(prompt); ok {
			review.Prompts = append(review.Prompts, item)
		}
		if prompt.IsSynced() && prompt.AlternateContent == "" && !prompt.Builtin {
			review.RepositoryOnly = append(review.RepositoryOnly, prompt.Name)
		}
	}
	sort.Slice(review.Prompts, func(i, j int) bool { return review.Prompts[i].Name < review.Prompts[j].Name })
	sort.Strings(review.RepositoryOnly)
	return review, nil
}

func twoVersionPrompt(prompt *promptmodels.Prompt) (Conflict, bool) {
	if prompt.AlternateContent == "" || prompt.AlternateContent == prompt.Content {
		return Conflict{}, false
	}
	item := Conflict{Name: prompt.Name, Path: prompt.SourcePath}
	switch {
	case prompt.IsSynced():
		item.LocalContent, item.RepositoryContent, item.InUse = prompt.AlternateContent, prompt.Content, promptservice.ChoiceRepository
	case prompt.IsOverride():
		item.LocalContent, item.RepositoryContent, item.InUse = prompt.Content, prompt.AlternateContent, promptservice.ChoiceLocal
	default:
		return Conflict{}, false
	}
	return item, true
}

// switchingPrompts names the prompts whose choice differs from the version
// they use now, so releasing them takes their other version.
func switchingPrompts(prompts []*promptmodels.Prompt, choices map[string]promptservice.ConflictChoice) []string {
	var names []string
	for _, prompt := range prompts {
		item, ok := twoVersionPrompt(prompt)
		if !ok {
			continue
		}
		if choice, chosen := choices[prompt.Name]; chosen && choice != item.InUse {
			names = append(names, prompt.Name)
		}
	}
	return names
}
