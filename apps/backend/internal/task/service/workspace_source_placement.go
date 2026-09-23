package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

// WorkspaceRepositoryPlacement is the closed set of destinations offered by
// the explicit workspace source attachment flow.
type WorkspaceRepositoryPlacement string

const (
	WorkspacePlacementKandevDirectory WorkspaceRepositoryPlacement = "kandev_directory"
	WorkspacePlacementCurrentRoot     WorkspaceRepositoryPlacement = "current_root"
	WorkspacePlacementExpandRoot      WorkspaceRepositoryPlacement = "expand_root"
)

var (
	ErrInvalidWorkspaceRepositoryPlacement = errors.New("invalid workspace repository placement")
	ErrWorkspaceSourcePreviewStale         = errors.New("workspace source preview is stale")
	ErrWorkspaceExpansionUnavailable       = errors.New("workspace expansion requires explicit session recovery")
)

type WorkspaceRepositoryPlacementOption struct {
	Placement WorkspaceRepositoryPlacement `json:"placement"`
	Enabled   bool                         `json:"enabled"`
	Reason    string                       `json:"reason,omitempty"`
}

type WorkspaceRepositoryPreviewSource struct {
	Kind                  WorkspaceSourceKind `json:"kind"`
	RepositoryID          string              `json:"repository_id"`
	RepositoryName        string              `json:"repository_name"`
	SourceName            string              `json:"source_name"`
	WorkspaceRelativePath string              `json:"workspace_relative_path"`
}

type WorkspaceRepositoryPlacementPreview struct {
	TaskID              string                               `json:"task_id"`
	Revision            string                               `json:"revision"`
	WorkspacePath       string                               `json:"workspace_path"`
	Placement           WorkspaceRepositoryPlacement         `json:"placement,omitempty"`
	Sources             []WorkspaceRepositoryPreviewSource   `json:"sources"`
	SupportedPlacements []WorkspaceRepositoryPlacementOption `json:"supported_placements"`
}

type workspaceRepositoryPlacementTarget struct {
	repository *models.TaskRepository
	folder     *models.TaskWorkspaceFolder
	relative   string
}

func validateWorkspaceRepositoryPlacement(placement WorkspaceRepositoryPlacement) error {
	switch placement {
	case WorkspacePlacementKandevDirectory, WorkspacePlacementCurrentRoot, WorkspacePlacementExpandRoot:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidWorkspaceRepositoryPlacement, placement)
	}
}

func (s *Service) applyWorkspaceRepositoryPlacement(
	ctx context.Context,
	task *models.Task,
	batch *models.WorkspaceSourceBatch,
	placement WorkspaceRepositoryPlacement,
	expectedRevision string,
) error {
	if placement == "" {
		return nil
	}
	if err := validateWorkspaceRepositoryPlacementBatch(batch, placement); err != nil {
		return err
	}
	env, err := s.workspaceRepositoryPlacementEnvironment(ctx, task.ID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(expectedRevision) == "" {
		return fmt.Errorf("%w: a current workspace preview is required", ErrWorkspaceSourcePreviewStale)
	}
	revision, revisionErr := s.workspaceRepositoryPlacementRevision(ctx, task.ID, env)
	if revisionErr != nil {
		return revisionErr
	}
	if revision != expectedRevision {
		return fmt.Errorf("%w: refresh the workspace preview", ErrWorkspaceSourcePreviewStale)
	}
	if workspaceRepositoryPlacementHasFolders(batch) && !isLocalWorkspaceExecutor(env.ExecutorType) {
		return fmt.Errorf("%w: folder placement requires a Local environment", ErrUnsupportedWorkspaceSource)
	}
	placements, paths, err := s.buildWorkspaceRepositoryPlacementTargets(ctx, task.WorkspaceID, env, batch.Sources, placement)
	if err != nil {
		return err
	}
	if err := preflightWorkspaceRepositoryDestinations(paths); err != nil {
		return err
	}
	for _, target := range placements {
		if target.repository != nil {
			target.repository.WorkspaceRelativePath = target.relative
		}
		if target.folder != nil {
			target.folder.WorkspaceRelativePath = target.relative
		}
	}
	batch.RepositoryPlacement = string(placement)
	batch.PreviewRevision = expectedRevision
	return nil
}

func validateWorkspaceRepositoryPlacementBatch(batch *models.WorkspaceSourceBatch, placement WorkspaceRepositoryPlacement) error {
	if err := validateWorkspaceRepositoryPlacement(placement); err != nil {
		return err
	}
	if placement == WorkspacePlacementExpandRoot {
		return ErrWorkspaceExpansionUnavailable
	}
	if batch == nil || len(batch.Sources) == 0 {
		return fmt.Errorf("%w: placement requires workspace sources", ErrInvalidWorkspaceRepositoryPlacement)
	}
	if len(batch.RepositoryUpdates) > 0 {
		return fmt.Errorf("%w: placement cannot be combined with branch updates", ErrInvalidWorkspaceRepositoryPlacement)
	}
	for _, source := range batch.Sources {
		if source.Repository == nil && source.Folder == nil {
			return fmt.Errorf("%w: placement requires a repository or folder source", ErrInvalidWorkspaceRepositoryPlacement)
		}
	}
	return nil
}

func (s *Service) workspaceRepositoryPlacementEnvironment(ctx context.Context, taskID string) (*models.TaskEnvironment, error) {
	if s.taskEnvironments == nil {
		return nil, fmt.Errorf("%w: task environment persistence is unavailable", ErrWorkspaceSourceMaterialize)
	}
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if env == nil || (!isLocalWorkspaceExecutor(env.ExecutorType) && env.ExecutorType != string(models.ExecutorTypeWorktree)) {
		return nil, fmt.Errorf("%w: explicit placement requires a Local or Worktree environment", ErrUnsupportedWorkspaceSource)
	}
	if env.WorkspacePath == "" || (env.ExecutorType == string(models.ExecutorTypeWorktree) && env.TaskDirName == "") {
		return nil, fmt.Errorf("%w: the task workspace root is not ready", ErrInvalidWorkspaceRepositoryPlacement)
	}
	return env, nil
}

func (s *Service) buildWorkspaceRepositoryPlacementTargets(ctx context.Context, workspaceID string, env *models.TaskEnvironment, sources []models.WorkspaceSource, placement WorkspaceRepositoryPlacement) ([]workspaceRepositoryPlacementTarget, []string, error) {
	targets := make([]workspaceRepositoryPlacementTarget, 0, len(sources))
	paths := make([]string, 0, len(sources))
	for _, source := range sources {
		var target workspaceRepositoryPlacementTarget
		var path string
		var err error
		switch {
		case source.Repository != nil && source.Folder == nil:
			target, path, err = s.buildWorkspaceRepositoryPlacementTarget(ctx, workspaceID, env, source.Repository, placement)
		case source.Folder != nil && source.Repository == nil:
			if !isLocalWorkspaceExecutor(env.ExecutorType) {
				return nil, nil, fmt.Errorf("%w: folders are unavailable for executor %q", ErrUnsupportedWorkspaceSource, env.ExecutorType)
			}
			relative, relativeErr := workspaceFolderRelativePath(env, placement, source.Folder.DisplayName)
			if relativeErr != nil {
				return nil, nil, relativeErr
			}
			path, err = workspaceRepositoryPlacementPath(env, relative)
			target = workspaceRepositoryPlacementTarget{folder: source.Folder, relative: relative}
		default:
			return nil, nil, fmt.Errorf("%w: workspace source kind is required", ErrInvalidWorkspaceRepositoryPlacement)
		}
		if err != nil {
			return nil, nil, err
		}
		targets = append(targets, target)
		paths = append(paths, path)
	}
	return targets, paths, nil
}

func (s *Service) buildWorkspaceRepositoryPlacementTarget(ctx context.Context, workspaceID string, env *models.TaskEnvironment, tr *models.TaskRepository, placement WorkspaceRepositoryPlacement) (workspaceRepositoryPlacementTarget, string, error) {
	entity, err := s.repositoryEntityInWorkspace(ctx, workspaceID, tr.RepositoryID)
	if err != nil {
		return workspaceRepositoryPlacementTarget{}, "", err
	}
	name, err := WorkspaceSourceRuntimeEntryName(env.ExecutorType, entity, tr)
	if err != nil {
		return workspaceRepositoryPlacementTarget{}, "", err
	}
	relative, err := workspaceRepositoryRelativePath(env, placement, name)
	if err != nil {
		return workspaceRepositoryPlacementTarget{}, "", err
	}
	path, err := workspaceRepositoryPlacementPath(env, relative)
	if err != nil {
		return workspaceRepositoryPlacementTarget{}, "", err
	}
	return workspaceRepositoryPlacementTarget{repository: tr, relative: relative}, path, nil
}

func workspaceFolderRelativePath(env *models.TaskEnvironment, placement WorkspaceRepositoryPlacement, name string) (string, error) {
	if env == nil || !isLocalWorkspaceExecutor(env.ExecutorType) || name == "" || filepath.Base(name) != name || worktree.SanitizeRepoDirName(name) != name {
		return "", fmt.Errorf("%w: unsafe folder entry", ErrInvalidWorkspaceRepositoryPlacement)
	}
	switch placement {
	case WorkspacePlacementKandevDirectory:
		return filepath.ToSlash(filepath.Join("kandev", name)), nil
	case WorkspacePlacementCurrentRoot:
		return filepath.ToSlash(name), nil
	case WorkspacePlacementExpandRoot:
		return "", ErrWorkspaceExpansionUnavailable
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidWorkspaceRepositoryPlacement, placement)
	}
}

func workspaceRepositoryRelativePath(env *models.TaskEnvironment, placement WorkspaceRepositoryPlacement, entry string) (string, error) {
	if env == nil || entry == "" || filepath.Base(entry) != entry || worktree.SanitizeRepoDirName(entry) != entry {
		return "", fmt.Errorf("%w: unsafe repository entry", ErrInvalidWorkspaceRepositoryPlacement)
	}
	layout := EffectiveTaskEnvironmentWorkspaceLayout(env)
	if layout == WorkspaceLayoutTaskRoot && placement == WorkspacePlacementKandevDirectory {
		return "", fmt.Errorf("%w: the task already uses the parent workspace layout", ErrInvalidWorkspaceRepositoryPlacement)
	}
	if isLocalWorkspaceExecutor(env.ExecutorType) {
		return localWorkspaceRepositoryRelativePath(placement, entry)
	}
	return remoteWorkspaceRepositoryRelativePath(env.WorkspacePath, layout, placement, entry)
}

func localWorkspaceRepositoryRelativePath(placement WorkspaceRepositoryPlacement, entry string) (string, error) {
	switch placement {
	case WorkspacePlacementKandevDirectory:
		return filepath.ToSlash(filepath.Join("kandev", entry)), nil
	case WorkspacePlacementCurrentRoot:
		return filepath.ToSlash(entry), nil
	case WorkspacePlacementExpandRoot:
		return "", ErrWorkspaceExpansionUnavailable
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidWorkspaceRepositoryPlacement, placement)
	}
}

func remoteWorkspaceRepositoryRelativePath(workspacePath, layout string, placement WorkspaceRepositoryPlacement, entry string) (string, error) {
	rootEntry := ""
	if layout != WorkspaceLayoutTaskRoot {
		rootEntry = filepath.Base(filepath.Clean(workspacePath))
		if rootEntry == "." || rootEntry == string(filepath.Separator) || rootEntry == "" {
			return "", fmt.Errorf("%w: cannot identify current workspace root", ErrInvalidWorkspaceRepositoryPlacement)
		}
	}
	var relative string
	switch placement {
	case WorkspacePlacementKandevDirectory:
		relative = filepath.Join(rootEntry, "kandev", entry)
	case WorkspacePlacementCurrentRoot:
		relative = filepath.Join(rootEntry, entry)
	case WorkspacePlacementExpandRoot:
		relative = entry
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidWorkspaceRepositoryPlacement, placement)
	}
	return filepath.ToSlash(relative), nil
}

// EffectiveTaskEnvironmentWorkspaceLayout returns the current physical layout,
// including legacy environments that predate the persisted workspace_layout.
func EffectiveTaskEnvironmentWorkspaceLayout(env *models.TaskEnvironment) string {
	if env == nil {
		return WorkspaceLayoutRepository
	}
	workspacePath := filepath.Clean(env.WorkspacePath)
	hasChildWorktree := false
	for _, repo := range env.Repos {
		if repo == nil || repo.WorktreePath == "" {
			continue
		}
		worktreePath := filepath.Clean(repo.WorktreePath)
		if worktreePath == workspacePath {
			return WorkspaceLayoutRepository
		}
		if filepath.Dir(worktreePath) == workspacePath {
			hasChildWorktree = true
		}
	}
	if hasChildWorktree {
		return WorkspaceLayoutTaskRoot
	}
	if env.TaskDirName != "" && filepath.Base(filepath.Clean(env.WorkspacePath)) == env.TaskDirName {
		return WorkspaceLayoutTaskRoot
	}
	if env.WorkspaceLayout != "" {
		return env.WorkspaceLayout
	}
	if isLocalWorkspaceExecutor(env.ExecutorType) && env.TaskDirName == "" && env.WorkspacePath != "" {
		return WorkspaceLayoutCurrentRoot
	}
	return WorkspaceLayoutRepository
}

func workspaceRepositoryPlacementPath(env *models.TaskEnvironment, relative string) (string, error) {
	if env == nil || env.WorkspacePath == "" || relative == "" {
		return "", fmt.Errorf("%w: workspace destination is not available", ErrInvalidWorkspaceRepositoryPlacement)
	}
	root := env.WorkspacePath
	layout := EffectiveTaskEnvironmentWorkspaceLayout(env)
	if layout != WorkspaceLayoutTaskRoot && !isLocalWorkspaceExecutor(env.ExecutorType) {
		root = filepath.Dir(root)
	}
	relativePath := filepath.Clean(filepath.FromSlash(relative))
	if relativePath == "." || filepath.IsAbs(relativePath) || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: unsafe workspace destination %q", ErrInvalidWorkspaceRepositoryPlacement, relative)
	}
	target := filepath.Join(root, relativePath)
	contained, err := filepath.Rel(root, target)
	if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: workspace destination escapes task root", ErrInvalidWorkspaceRepositoryPlacement)
	}
	return target, nil
}

func preflightWorkspaceRepositoryDestinations(paths []string) error {
	seen := make(map[string]string, len(paths))
	for _, path := range paths {
		if path == "" {
			return fmt.Errorf("%w: workspace destination is empty", ErrInvalidWorkspaceRepositoryPlacement)
		}
		key := strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
		if previous, found := seen[key]; found {
			return workspaceRepositoryPlacementConflict(fmt.Sprintf("%q collides with %q", path, previous))
		}
		seen[key] = path
		if _, err := os.Lstat(path); err == nil {
			return workspaceRepositoryPlacementConflict(fmt.Sprintf("%q already exists", path))
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: inspect workspace destination %q: %v", ErrInvalidWorkspaceRepositoryPlacement, path, err)
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%w: inspect workspace destination directory %q: %v", ErrInvalidWorkspaceRepositoryPlacement, filepath.Dir(path), err)
		}
		name := filepath.Base(path)
		for _, entry := range entries {
			if entry.Name() != name && strings.EqualFold(entry.Name(), name) {
				return workspaceRepositoryPlacementConflict(fmt.Sprintf("%q differs from existing entry %q only by case", name, entry.Name()))
			}
		}
	}
	return nil
}

func workspaceRepositoryPlacementConflict(detail string) error {
	return fmt.Errorf("%w: %w: %s", ErrWorkspaceSourceConflict, worktree.ErrWorkspacePathOccupied, detail)
}

func (s *Service) PreviewWorkspaceRepositoryPlacement(ctx context.Context, taskID string, sources []WorkspaceSourceInput, placement WorkspaceRepositoryPlacement) (*WorkspaceRepositoryPlacementPreview, error) {
	if err := validateWorkspaceRepositoryPlacementPreviewRequest(taskID, sources, placement); err != nil {
		return nil, err
	}
	if err := s.authorizeTaskID(ctx, taskID); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("%w: %s", repository.ErrTaskNotFound, taskID)
	}
	env, err := s.workspaceRepositoryPlacementEnvironment(ctx, taskID)
	if err != nil {
		return nil, err
	}
	layout := EffectiveTaskEnvironmentWorkspaceLayout(env)
	if hasWorkspaceFolderSourceInput(sources) && !isLocalWorkspaceExecutor(env.ExecutorType) {
		return nil, fmt.Errorf("%w: folders are unavailable for executor %q", ErrUnsupportedWorkspaceSource, env.ExecutorType)
	}
	if placement == WorkspacePlacementKandevDirectory && layout == WorkspaceLayoutTaskRoot {
		return nil, fmt.Errorf("%w: the task already uses the parent workspace layout", ErrInvalidWorkspaceRepositoryPlacement)
	}
	preview := &WorkspaceRepositoryPlacementPreview{
		TaskID:        taskID,
		WorkspacePath: env.WorkspacePath,
		Placement:     placement,
		Sources:       make([]WorkspaceRepositoryPreviewSource, 0, len(sources)),
		SupportedPlacements: []WorkspaceRepositoryPlacementOption{
			{Placement: WorkspacePlacementKandevDirectory, Enabled: layout != WorkspaceLayoutTaskRoot, Reason: placementUnavailableReason(layout != WorkspaceLayoutTaskRoot, "the task already uses the parent workspace layout")},
			{Placement: WorkspacePlacementCurrentRoot, Enabled: true},
			{Placement: WorkspacePlacementExpandRoot, Enabled: false, Reason: "explicit session recovery is required before expanding the workspace root"},
		},
	}
	previewPlacement := placement
	if previewPlacement == "" {
		// The initial request only asks which destinations are available. Use the
		// current root to render illustrative source paths, and defer collision
		// checks until the user selects a destination.
		previewPlacement = WorkspacePlacementCurrentRoot
	}
	previewSources, paths, err := s.buildWorkspaceRepositoryPlacementPreviewSources(ctx, task.WorkspaceID, env, sources, previewPlacement)
	if err != nil {
		return nil, err
	}
	preview.Sources = previewSources
	if placement != "" {
		if err := preflightWorkspaceRepositoryDestinations(paths); err != nil {
			return nil, err
		}
	}
	preview.Revision, err = s.workspaceRepositoryPlacementRevision(ctx, taskID, env)
	if err != nil {
		return nil, err
	}
	return preview, nil
}

func validateWorkspaceRepositoryPlacementPreviewRequest(taskID string, sources []WorkspaceSourceInput, placement WorkspaceRepositoryPlacement) error {
	if taskID == "" || len(sources) == 0 {
		return fmt.Errorf("%w: task_id and sources are required", ErrInvalidWorkspaceSource)
	}
	if placement != "" {
		if err := validateWorkspaceRepositoryPlacement(placement); err != nil {
			return err
		}
	}
	if placement == WorkspacePlacementExpandRoot {
		return ErrWorkspaceExpansionUnavailable
	}
	return nil
}

func (s *Service) buildWorkspaceRepositoryPlacementPreviewSources(ctx context.Context, workspaceID string, env *models.TaskEnvironment, sources []WorkspaceSourceInput, placement WorkspaceRepositoryPlacement) ([]WorkspaceRepositoryPreviewSource, []string, error) {
	previewSources := make([]WorkspaceRepositoryPreviewSource, 0, len(sources))
	paths := make([]string, 0, len(sources))
	for _, source := range sources {
		previewSource, path, err := s.buildWorkspaceRepositoryPlacementPreviewSource(ctx, workspaceID, env, source, placement)
		if err != nil {
			return nil, nil, err
		}
		paths = append(paths, path)
		previewSources = append(previewSources, previewSource)
	}
	return previewSources, paths, nil
}

func (s *Service) buildWorkspaceRepositoryPlacementPreviewSource(ctx context.Context, workspaceID string, env *models.TaskEnvironment, source WorkspaceSourceInput, placement WorkspaceRepositoryPlacement) (WorkspaceRepositoryPreviewSource, string, error) {
	switch source.Kind {
	case WorkspaceSourceRepository:
		if source.RepositoryID == "" || repositoryLocatorCount(source) != 1 {
			return WorkspaceRepositoryPreviewSource{}, "", fmt.Errorf("%w: preview requires existing repository IDs", ErrInvalidWorkspaceSource)
		}
		entity, err := s.repositoryEntityInWorkspace(ctx, workspaceID, source.RepositoryID)
		if err != nil {
			return WorkspaceRepositoryPreviewSource{}, "", err
		}
		tr := &models.TaskRepository{RepositoryID: source.RepositoryID, BaseBranch: source.BaseBranch, CheckoutBranch: source.CheckoutBranch}
		if tr.BaseBranch == "" {
			tr.BaseBranch = entity.DefaultBranch
		}
		target, path, err := s.buildWorkspaceRepositoryPlacementTarget(ctx, workspaceID, env, tr, placement)
		if err != nil {
			return WorkspaceRepositoryPreviewSource{}, "", err
		}
		return WorkspaceRepositoryPreviewSource{Kind: WorkspaceSourceRepository, RepositoryID: entity.ID, RepositoryName: entity.Name, SourceName: entity.Name, WorkspaceRelativePath: target.relative}, path, nil
	case WorkspaceSourceFolder:
		if !isLocalWorkspaceExecutor(env.ExecutorType) {
			return WorkspaceRepositoryPreviewSource{}, "", fmt.Errorf("%w: folders are unavailable for executor %q", ErrUnsupportedWorkspaceSource, env.ExecutorType)
		}
		path, err := canonicalFolder(source.LocalPath)
		if err != nil {
			return WorkspaceRepositoryPreviewSource{}, "", fmt.Errorf("%w: %v", ErrInvalidWorkspaceSource, err)
		}
		name := source.DisplayName
		if name == "" {
			name = filepath.Base(path)
		}
		relative, err := workspaceFolderRelativePath(env, placement, name)
		if err != nil {
			return WorkspaceRepositoryPreviewSource{}, "", err
		}
		destination, err := workspaceRepositoryPlacementPath(env, relative)
		if err != nil {
			return WorkspaceRepositoryPreviewSource{}, "", err
		}
		return WorkspaceRepositoryPreviewSource{Kind: WorkspaceSourceFolder, RepositoryName: name, SourceName: name, WorkspaceRelativePath: relative}, destination, nil
	default:
		return WorkspaceRepositoryPreviewSource{}, "", fmt.Errorf("%w: unsupported workspace source kind %q", ErrInvalidWorkspaceSource, source.Kind)
	}
}

func (s *Service) validateExactWorkspaceRepositoryPlacement(ctx context.Context, task *models.Task, sources []WorkspaceSourceInput, placement WorkspaceRepositoryPlacement) error {
	if err := validateWorkspaceRepositoryPlacement(placement); err != nil {
		return err
	}
	if placement == WorkspacePlacementExpandRoot {
		return ErrWorkspaceExpansionUnavailable
	}
	env, existing, err := s.exactWorkspaceRepositoryPlacementState(ctx, task.ID)
	if err != nil {
		return err
	}
	for _, source := range sources {
		if err := s.validateExactWorkspaceRepositorySource(ctx, task.WorkspaceID, env, existing, source, placement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) exactWorkspaceRepositoryPlacementState(ctx context.Context, taskID string) (*models.TaskEnvironment, []*models.TaskRepository, error) {
	if s.taskEnvironments == nil {
		return nil, nil, fmt.Errorf("%w: task environment persistence is unavailable", ErrWorkspaceSourceMaterialize)
	}
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	if env == nil || (!isLocalWorkspaceExecutor(env.ExecutorType) && env.ExecutorType != string(models.ExecutorTypeWorktree)) || env.WorkspacePath == "" || (env.ExecutorType == string(models.ExecutorTypeWorktree) && env.TaskDirName == "") {
		return nil, nil, fmt.Errorf("%w: the task workspace root is not ready", ErrInvalidWorkspaceRepositoryPlacement)
	}
	existing, err := s.taskRepos.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	return env, existing, nil
}

func (s *Service) validateExactWorkspaceRepositorySource(ctx context.Context, workspaceID string, env *models.TaskEnvironment, existing []*models.TaskRepository, source WorkspaceSourceInput, placement WorkspaceRepositoryPlacement) error {
	if source.Kind != WorkspaceSourceRepository || source.RepositoryID == "" || repositoryLocatorCount(source) != 1 {
		return fmt.Errorf("%w: exact placement retries require repository IDs", ErrInvalidWorkspaceSource)
	}
	entity, err := s.repositoryEntityInWorkspace(ctx, workspaceID, source.RepositoryID)
	if err != nil {
		return err
	}
	base := source.BaseBranch
	if base == "" {
		base = entity.DefaultBranch
	}
	matched := findExactWorkspaceRepository(existing, source.RepositoryID, base, source.CheckoutBranch)
	if matched == nil {
		return fmt.Errorf("%w: source was not already attached", ErrWorkspaceSourcePreviewStale)
	}
	name, err := WorkspaceSourceRuntimeEntryName(env.ExecutorType, entity, matched)
	if err != nil {
		return err
	}
	expected, err := workspaceRepositoryRelativePath(env, placement, name)
	if err != nil {
		return err
	}
	if matched.WorkspaceRelativePath != expected {
		return fmt.Errorf("%w: refresh the workspace preview", ErrWorkspaceSourcePreviewStale)
	}
	return nil
}

func workspaceRepositoryPlacementHasFolders(batch *models.WorkspaceSourceBatch) bool {
	if batch == nil {
		return false
	}
	return slicesContainsFolder(batch.Sources)
}

func slicesContainsFolder(sources []models.WorkspaceSource) bool {
	for _, source := range sources {
		if source.Folder != nil {
			return true
		}
	}
	return false
}

func hasWorkspaceFolderSourceInput(sources []WorkspaceSourceInput) bool {
	for _, source := range sources {
		if source.Kind == WorkspaceSourceFolder {
			return true
		}
	}
	return false
}

func (s *Service) repositoryEntityInWorkspace(ctx context.Context, workspaceID, repositoryID string) (*models.Repository, error) {
	entity, err := s.repoEntities.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("%w: repository %q", repository.ErrRepositoryNotFound, repositoryID)
	}
	if entity.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("%w: repository %q does not belong to workspace %q", ErrTaskReferenceNotFound, repositoryID, workspaceID)
	}
	return entity, nil
}

func findExactWorkspaceRepository(existing []*models.TaskRepository, repositoryID, baseBranch, checkoutBranch string) *models.TaskRepository {
	for _, candidate := range existing {
		if candidate != nil && candidate.RepositoryID == repositoryID && candidate.BaseBranch == baseBranch && candidate.CheckoutBranch == checkoutBranch {
			return candidate
		}
	}
	return nil
}

func placementUnavailableReason(enabled bool, reason string) string {
	if enabled {
		return ""
	}
	return reason
}

func (s *Service) workspaceRepositoryPlacementRevision(ctx context.Context, taskID string, env *models.TaskEnvironment) (string, error) {
	sessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	values := []string{env.ID, fmt.Sprint(env.OwnershipGeneration), env.TaskDirName, env.WorkspacePath, env.WorkspaceLayout}
	repoRows := append([]*models.TaskEnvironmentRepo(nil), env.Repos...)
	sort.Slice(repoRows, func(i, j int) bool {
		return taskEnvironmentRepoRevisionKey(repoRows[i]) < taskEnvironmentRepoRevisionKey(repoRows[j])
	})
	for _, row := range repoRows {
		if row == nil {
			continue
		}
		values = append(values, row.RepositoryID, row.BranchSlug, row.WorktreeID, row.WorktreePath, row.WorkspaceRelativePath)
	}
	sessionRows := append([]*models.TaskSession(nil), sessions...)
	sort.Slice(sessionRows, func(i, j int) bool {
		return taskSessionRevisionKey(sessionRows[i]) < taskSessionRevisionKey(sessionRows[j])
	})
	for _, session := range sessionRows {
		if session != nil {
			values = append(values, session.ID, session.TaskEnvironmentID, session.WorkspacePath, string(session.State))
		}
	}
	if s.workspaceFolders != nil {
		folders, err := s.workspaceFolders.ListTaskWorkspaceFolders(ctx, taskID)
		if err != nil {
			return "", err
		}
		folderRows := append([]*models.TaskWorkspaceFolder(nil), folders...)
		sort.Slice(folderRows, func(i, j int) bool {
			return taskWorkspaceFolderRevisionKey(folderRows[i]) < taskWorkspaceFolderRevisionKey(folderRows[j])
		})
		for _, folder := range folderRows {
			if folder != nil {
				values = append(values, folder.ID, folder.LocalPath, folder.DisplayName, folder.WorkspaceRelativePath)
			}
		}
	}
	_, _ = h.Write([]byte(strings.Join(values, "\x00")))
	return hex.EncodeToString(h.Sum(nil)), nil
}

func taskEnvironmentRepoRevisionKey(row *models.TaskEnvironmentRepo) string {
	if row == nil {
		return ""
	}
	return strings.Join([]string{row.RepositoryID, row.BranchSlug, row.WorktreeID, row.WorktreePath, row.WorkspaceRelativePath}, "\x00")
}

func taskSessionRevisionKey(session *models.TaskSession) string {
	if session == nil {
		return ""
	}
	return strings.Join([]string{session.ID, session.TaskEnvironmentID, session.WorkspacePath, string(session.State)}, "\x00")
}

func taskWorkspaceFolderRevisionKey(folder *models.TaskWorkspaceFolder) string {
	if folder == nil {
		return ""
	}
	return strings.Join([]string{folder.ID, folder.LocalPath, folder.DisplayName, folder.WorkspaceRelativePath}, "\x00")
}
