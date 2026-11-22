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
	// Resolutions are the user's choices for name conflicts, by prompt name.
	// A conflict without a choice is adopted as a conflicted override.
	Resolutions map[string]ConflictChoice
}

// ConflictChoice is the user's side in a name conflict between a repository
// file and a manual, built-in, or conflicted override prompt.
type ConflictChoice string

const (
	// ChoiceLocal keeps the existing content as an override of the file.
	ChoiceLocal ConflictChoice = "local"
	// ChoiceRepository uses the file's content and keeps the existing content
	// as the local version.
	ChoiceRepository ConflictChoice = "repository"
)

// Valid reports whether c is a known choice.
func (c ConflictChoice) Valid() bool {
	return c == ChoiceLocal || c == ChoiceRepository
}

// SyncApplyResult reports what one sync run changed. Overridden lists files
// whose prompt is overridden locally. Conflicts lists overrides still waiting
// for the user to pick a side. Restored lists synced prompts that left the
// source and became manual again with their kept or current content.
type SyncApplyResult struct {
	Created    []string
	Updated    []string
	Deleted    []string
	Restored   []string
	Overridden []string
	Conflicts  []string
	Warnings   []string
	// wrote records writes that are not content changes (ownership, or an
	// override's stored repository version), so the change event still
	// reaches open prompt lists.
	wrote bool
}

// Changed reports whether the run wrote anything, ownership changes included.
func (r *SyncApplyResult) Changed() bool {
	return r.wrote || len(r.Created)+len(r.Updated)+len(r.Deleted)+len(r.Restored) > 0
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
	// Writes are individual statements, so a later failure can leave earlier
	// ones saved; publish whenever anything was written, including on error.
	defer func() {
		if result.Changed() {
			s.publishChanged(ctx)
		}
	}()
	desired := make(map[string]bool, len(input.Desired))
	for _, file := range input.Desired {
		desired[file.Name] = true
		if err := s.applySyncedFile(ctx, file, byName[file.Name], input.Resolutions[file.Name], result); err != nil {
			return nil, err
		}
	}
	if err := s.removeVanishedSyncedPrompts(ctx, existing, desired, input, result); err != nil {
		return nil, err
	}
	for _, names := range [][]string{
		result.Created, result.Updated, result.Deleted, result.Restored, result.Overridden, result.Conflicts,
	} {
		sort.Strings(names)
	}
	return result, nil
}

func (s *Service) applySyncedFile(
	ctx context.Context, file SyncedPromptFile, current *models.Prompt, choice ConflictChoice, result *SyncApplyResult,
) error {
	if current != nil && current.Content == file.Content && (!current.IsOverride() || current.Conflict) {
		// Identical text leaves nothing to choose between.
		choice = ChoiceRepository
	}
	switch {
	case current == nil:
		return s.createSyncedFile(ctx, file, result)
	case current.IsOverride():
		return s.applyToOverride(ctx, file, current, choice, result)
	case !current.IsSynced():
		return s.adoptClashingPrompt(ctx, file, current, choice, result)
	case current.Content == file.Content && current.SourcePath == file.Path:
		return nil
	default:
		return s.updateSyncedFile(ctx, current.ID, file, result)
	}
}

func (s *Service) createSyncedFile(ctx context.Context, file SyncedPromptFile, result *SyncApplyResult) error {
	_, err := s.repo.CreateSyncedPrompt(ctx, file.Name, file.Content, file.Path)
	switch {
	case err == nil:
		result.Created = append(result.Created, file.Name)
		return nil
	case translateNameConflict(err) == ErrPromptAlreadyExists:
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s: a prompt named %q was created during this sync; sync again to review it", file.Path, file.Name))
		return nil
	case errors.Is(err, promptstore.ErrPromptListLimit):
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s: prompt %q was not created because the saved prompt limit is reached", file.Path, file.Name))
		return nil
	default:
		return err
	}
}

func (s *Service) updateSyncedFile(ctx context.Context, id string, file SyncedPromptFile, result *SyncApplyResult) error {
	updated, err := s.repo.UpdateSyncedPrompt(ctx, id, file.Content, file.Path)
	if err != nil {
		return err
	}
	if updated {
		result.Updated = append(result.Updated, file.Name)
	}
	return nil
}

// applyToOverride leaves an override's content alone unless the user chose
// the repository version for it; a conflicted override stays reported until
// the user picks a side.
func (s *Service) applyToOverride(
	ctx context.Context, file SyncedPromptFile, current *models.Prompt, choice ConflictChoice, result *SyncApplyResult,
) error {
	switch {
	case choice == ChoiceRepository:
		reverted, err := s.repo.RevertOverride(ctx, current.ID)
		if err != nil || !reverted {
			return err
		}
		result.wrote = true
		return s.updateSyncedFile(ctx, current.ID, file, result)
	case choice == ChoiceLocal && current.Conflict:
		cleared, err := s.repo.ClearOverrideConflict(ctx, current.ID)
		if err != nil {
			return err
		}
		result.wrote = result.wrote || cleared
	case current.Conflict:
		result.Conflicts = append(result.Conflicts, file.Name)
	}
	recorded, err := s.repo.SetOverrideRepositoryContent(ctx, current.ID, file.Content)
	if err != nil {
		return err
	}
	result.wrote = result.wrote || recorded
	result.Overridden = append(result.Overridden, file.Name)
	return nil
}

// adoptClashingPrompt brings a manual or built-in prompt whose name the file
// also uses under sync, following the user's choice; without one it becomes
// a conflicted override that keeps its content.
func (s *Service) adoptClashingPrompt(
	ctx context.Context, file SyncedPromptFile, current *models.Prompt, choice ConflictChoice, result *SyncApplyResult,
) error {
	if choice == ChoiceRepository {
		adopted, err := s.repo.AdoptPromptAsSynced(ctx, current, file.Content, file.Path)
		if err != nil {
			return err
		}
		if adopted {
			result.Updated = append(result.Updated, file.Name)
		}
		return nil
	}
	conflict := choice != ChoiceLocal
	adopted, err := s.repo.AdoptPromptAsOverride(ctx, current, file.Path, file.Content, conflict)
	if err != nil || !adopted {
		return err
	}
	result.wrote = true
	result.Overridden = append(result.Overridden, file.Name)
	if conflict {
		result.Conflicts = append(result.Conflicts, file.Name)
	}
	return nil
}

func (s *Service) removeVanishedSyncedPrompts(
	ctx context.Context, existing []*models.Prompt, desired map[string]bool, input SyncApplyInput, result *SyncApplyResult,
) error {
	var vanished []*models.Prompt
	for _, prompt := range existing {
		if desired[prompt.Name] || input.FrozenNames[prompt.Name] {
			continue
		}
		kept, err := s.settleVanishedPrompt(ctx, prompt, result)
		if err != nil {
			return err
		}
		if !kept {
			vanished = append(vanished, prompt)
		}
	}
	if len(vanished) == 0 {
		return nil
	}
	survivors, err := s.survivingPrompts(ctx, vanished)
	if err != nil {
		return err
	}
	references, err := s.vanishedPromptReferences(ctx, vanished, survivors, input.ReferenceTexts)
	if err != nil {
		return err
	}
	for _, prompt := range vanished {
		if count := references[prompt.Name]; count > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"prompt %q was removed from the sync source but is still referenced by %d workflow(s) or saved prompt(s); it was kept. Remove the references, then sync again",
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

// survivingPrompts reads every prompt except the vanished ones after this
// run's writes, so references count the text each prompt has now, including
// prompts this run created or updated.
func (s *Service) survivingPrompts(ctx context.Context, vanished []*models.Prompt) ([]*models.Prompt, error) {
	current, err := s.repo.ListPrompts(ctx)
	if err != nil {
		return nil, err
	}
	gone := make(map[string]bool, len(vanished))
	for _, prompt := range vanished {
		gone[prompt.ID] = true
	}
	survivors := current[:0]
	for _, prompt := range current {
		if !gone[prompt.ID] {
			survivors = append(survivors, prompt)
		}
	}
	return survivors, nil
}

// settleVanishedPrompt handles a prompt whose file left the source, and
// reports whether it stays without the reference check: an override loses
// its conflict and repository version, and a synced prompt with a kept local
// version or a built-in prompt becomes manual instead of being deleted.
func (s *Service) settleVanishedPrompt(ctx context.Context, prompt *models.Prompt, result *SyncApplyResult) (bool, error) {
	switch {
	case prompt.IsOverride():
		detached, err := s.repo.DetachOverride(ctx, prompt.ID)
		result.wrote = result.wrote || detached
		return true, err
	case prompt.IsSynced() && (prompt.AlternateContent != "" || prompt.Builtin):
		restored, err := s.repo.RestoreSyncedPrompt(ctx, prompt.ID)
		if err == nil && restored {
			result.Restored = append(result.Restored, prompt.Name)
		}
		return true, err
	default:
		return !prompt.IsSynced(), nil
	}
}

// vanishedPromptReferences counts the referrers of each vanished prompt. A
// vanished prompt is kept when a workflow text or a surviving saved prompt
// references it; a kept prompt survives too, so its own references are
// followed until no further prompt is kept. Expansion follows nested
// references, so deleting a prompt reached only through another saved prompt
// would silently drop content a workflow depends on.
func (s *Service) vanishedPromptReferences(
	ctx context.Context, vanished, survivors []*models.Prompt, workflowTexts []PromptReferenceText,
) (map[string]int, error) {
	texts := append([]PromptReferenceText(nil), workflowTexts...)
	for _, prompt := range survivors {
		texts = append(texts, promptReferenceText(prompt))
	}
	pending := vanished
	counts := map[string]int{}
	for len(pending) > 0 {
		names := make([]string, len(pending))
		for i, prompt := range pending {
			names[i] = prompt.Name
		}
		found, err := s.referencedPromptNames(ctx, texts, names)
		if err != nil {
			return nil, err
		}
		var next []*models.Prompt
		for _, prompt := range pending {
			if found[prompt.Name] == 0 {
				next = append(next, prompt)
				continue
			}
			counts[prompt.Name] = found[prompt.Name]
			texts = append(texts, promptReferenceText(prompt))
		}
		if len(next) == len(pending) {
			break
		}
		pending = next
	}
	return counts, nil
}

func promptReferenceText(prompt *models.Prompt) PromptReferenceText {
	return PromptReferenceText{Owner: "prompt:" + prompt.Name, Text: prompt.Content}
}

// ReleaseSyncedPrompts converts every synced prompt and override into a
// manual, editable prompt, applying the user's removal choices, and returns
// how many prompts were released or deleted.
func (s *Service) ReleaseSyncedPrompts(ctx context.Context, opts promptstore.ReleaseOptions) (int64, error) {
	released, err := s.repo.ReleaseSyncedPrompts(ctx, opts)
	if err != nil {
		return 0, err
	}
	if released > 0 {
		s.publishChanged(ctx)
	}
	return released, nil
}
