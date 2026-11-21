package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/prompts/models"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
)

// ErrPromptNotLinked rejects a version save for a prompt that Prompt Sync
// does not manage, so it has no repository version.
var ErrPromptNotLinked = errors.New("only a synced or overridden prompt has a repository version")

// ErrNoRepositoryVersion rejects choosing the repository version of an
// override whose file left the sync source, so its local version is the only
// copy.
var ErrNoRepositoryVersion = errors.New("this prompt's file is no longer in the synced repository, so only your version exists")

// VersionSave is the operator's saved choice for a synced prompt or an
// override: which version is in use and, optionally, an edited local version.
type VersionSave struct {
	UseLocal          bool
	LocalContent      *string
	ExpectedUpdatedAt *time.Time
}

// SavePromptVersions applies the Local | Repository choice and any edit to the
// local version in one write. With UseLocal the prompt becomes an override
// using its local version (or the repository version when it has none);
// otherwise it is synced, using the repository version and keeping the local
// one. Either way neither version is lost and any conflict is resolved.
func (s *Service) SavePromptVersions(ctx context.Context, id string, save VersionSave) (*models.Prompt, error) {
	prompt, err := s.GetPromptByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !prompt.IsSynced() && !prompt.IsOverride() {
		return nil, ErrPromptNotLinked
	}
	if save.ExpectedUpdatedAt != nil && !prompt.UpdatedAt.Equal(*save.ExpectedUpdatedAt) {
		return nil, promptstore.ErrPromptWriteRejected
	}
	local, repository := prompt.AlternateContent, prompt.Content
	if prompt.IsOverride() {
		local, repository = prompt.Content, prompt.AlternateContent
	}
	if !save.UseLocal && repository == "" {
		return nil, ErrNoRepositoryVersion
	}
	if save.LocalContent != nil {
		trimmed := strings.TrimSpace(*save.LocalContent)
		if trimmed == "" || len(trimmed) > maxPromptContentBytes {
			return nil, ErrInvalidPrompt
		}
		local = trimmed
	}
	changed, err := s.repo.SavePromptVersions(ctx, id, promptstore.VersionsOf(prompt), nextVersions(save.UseLocal, local, repository))
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, promptstore.ErrPromptWriteRejected
	}
	s.publishChanged(ctx)
	return s.GetPromptByID(ctx, id)
}

// nextVersions places the chosen version in use and keeps the other one. A
// missing local version falls back to the repository version; a missing
// repository version is rejected before this point.
func nextVersions(useLocal bool, local, repository string) promptstore.PromptVersions {
	if useLocal {
		if local == "" {
			local = repository
		}
		return promptstore.PromptVersions{Source: models.PromptSourceOverride, Content: local, AlternateContent: repository}
	}
	return promptstore.PromptVersions{Source: models.PromptSourceSync, Content: repository, AlternateContent: local}
}
