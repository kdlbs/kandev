package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/prompts/models"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
)

// ErrPromptSyncManaged rejects edits to a prompt owned by Prompt Sync.
var ErrPromptSyncManaged = errors.New("this prompt is managed by Prompt Sync and is read-only; edit its file in the synced repository")

// SyncedPromptFile is one valid prompt file from the sync source.
type SyncedPromptFile struct {
	Name    string
	Content string
	Path    string
}

// SyncApplyInput is the desired synced-prompt state for one sync run.
type SyncApplyInput struct {
	Desired []SyncedPromptFile
	// FrozenNames are names whose source file was invalid this run; their
	// synced prompts stay unchanged and are not treated as removed.
	FrozenNames map[string]bool
	// ReferenceTexts are the workflow and step prompts that block deleting a
	// synced prompt they still reference.
	ReferenceTexts []PromptReferenceText
}

// SyncApplyResult reports what one sync run changed.
type SyncApplyResult struct {
	Created  []string
	Updated  []string
	Deleted  []string
	Warnings []string
}

func (r *SyncApplyResult) changed() bool {
	return len(r.Created)+len(r.Updated)+len(r.Deleted) > 0
}

// ApplySyncedPrompts reconciles synced prompts with the desired files.
// Missing prompts are created and changed ones updated; a removed synced
// prompt is deleted only when no reference text still uses it. Manual and
// built-in prompts are never modified. Every write is guarded by sync
// ownership in storage, so a concurrent manual change cannot be overwritten.
func (s *Service) ApplySyncedPrompts(ctx context.Context, input SyncApplyInput) (*SyncApplyResult, error) {
	existing, err := s.repo.ListPrompts(ctx)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]*models.Prompt, len(existing))
	for _, prompt := range existing {
		byName[prompt.Name] = prompt
	}
	result := &SyncApplyResult{}
	desired := make(map[string]bool, len(input.Desired))
	for _, file := range input.Desired {
		desired[file.Name] = true
		if err := s.applySyncedFile(ctx, file, byName[file.Name], result); err != nil {
			return nil, err
		}
	}
	if err := s.removeVanishedSyncedPrompts(ctx, existing, desired, input, result); err != nil {
		return nil, err
	}
	sort.Strings(result.Created)
	sort.Strings(result.Updated)
	sort.Strings(result.Deleted)
	if result.changed() {
		s.publishChanged(ctx)
	}
	return result, nil
}

func (s *Service) applySyncedFile(ctx context.Context, file SyncedPromptFile, current *models.Prompt, result *SyncApplyResult) error {
	switch {
	case current == nil:
		_, err := s.repo.CreateSyncedPrompt(ctx, file.Name, file.Content, file.Path)
		if err == nil {
			result.Created = append(result.Created, file.Name)
			return nil
		}
		if translateNameConflict(err) == ErrPromptAlreadyExists {
			result.Warnings = append(result.Warnings, clashWarning(file))
			return nil
		}
		if errors.Is(err, promptstore.ErrPromptListLimit) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: prompt %q was not created because the saved prompt limit is reached", file.Path, file.Name))
			return nil
		}
		return err
	case !current.IsSynced():
		result.Warnings = append(result.Warnings, clashWarning(file))
		return nil
	case current.Content == file.Content && current.SourcePath == file.Path:
		return nil
	default:
		updated, err := s.repo.UpdateSyncedPrompt(ctx, current.ID, file.Content, file.Path)
		if err != nil {
			return err
		}
		if updated {
			result.Updated = append(result.Updated, file.Name)
		}
		return nil
	}
}

func clashWarning(file SyncedPromptFile) string {
	return fmt.Sprintf("%s: a manual or built-in prompt named %q already exists; it was left unchanged and this file was skipped", file.Path, file.Name)
}

func (s *Service) removeVanishedSyncedPrompts(
	ctx context.Context, existing []*models.Prompt, desired map[string]bool, input SyncApplyInput, result *SyncApplyResult,
) error {
	var vanished []*models.Prompt
	var names []string
	for _, prompt := range existing {
		if !prompt.IsSynced() || desired[prompt.Name] || input.FrozenNames[prompt.Name] {
			continue
		}
		vanished = append(vanished, prompt)
		names = append(names, prompt.Name)
	}
	if len(vanished) == 0 {
		return nil
	}
	references, err := s.ReferencedPromptNames(ctx, input.ReferenceTexts, names)
	if err != nil {
		return err
	}
	for _, prompt := range vanished {
		if count := references[prompt.Name]; count > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"prompt %q was removed from the sync source but is still referenced by %d workflow(s); it was kept. Remove the references, then sync again",
				prompt.Name, count))
			continue
		}
		deleted, err := s.repo.DeleteSyncedPrompt(ctx, prompt.ID)
		if err != nil {
			return err
		}
		if deleted {
			result.Deleted = append(result.Deleted, prompt.Name)
		}
	}
	return nil
}

// ReleaseSyncedPrompts converts every synced prompt into a manual, editable
// prompt and returns how many were released.
func (s *Service) ReleaseSyncedPrompts(ctx context.Context) (int64, error) {
	released, err := s.repo.ReleaseSyncedPrompts(ctx)
	if err != nil {
		return 0, err
	}
	if released > 0 {
		s.publishChanged(ctx)
	}
	return released, nil
}
