package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Repository sets are named, reusable groups of workspace repositories that fill
// the task-creation picker in one action. They live here rather than in
// service_resources.go, which is already at its size limit.

const repositorySetNameMaxLength = 100

var (
	// ErrInvalidRepositorySet marks a malformed request: a blank or overlong
	// name, no members, or a repeated member. Maps to 400.
	ErrInvalidRepositorySet = errors.New("invalid repository set")
	// ErrRepositorySetNameConflict marks a name already used in the workspace,
	// compared case-insensitively. Maps to 409.
	ErrRepositorySetNameConflict = errors.New("repository set name already used")
	// ErrUnknownRepositorySetMembers marks members that do not resolve to a live
	// repository in the set's workspace. Maps to 422. A cross-workspace id is
	// reported the same way as a nonexistent one, so membership validation does
	// not reveal repositories the caller cannot see.
	ErrUnknownRepositorySetMembers = errors.New("unknown repository set members")
)

// RepositorySetMemberInput is the wire/service input for one ordered set
// member. BaseBranch is optional; an empty value preserves task-form defaults.
type RepositorySetMemberInput struct {
	RepositoryID string `json:"repository_id"`
	BaseBranch   string `json:"base_branch,omitempty"`
}

// CreateRepositorySetRequest creates a set. Repositories is the new ordered
// member shape. RepositoryIDs remains the no-base compatibility input.
type CreateRepositorySetRequest struct {
	WorkspaceID   string                     `json:"workspace_id"`
	Name          string                     `json:"name"`
	Description   string                     `json:"description"`
	RepositoryIDs []string                   `json:"repository_ids"`
	Repositories  []RepositorySetMemberInput `json:"repositories"`
}

// UpdateRepositorySetRequest patches a set. Every field is optional; an absent
// field is left alone. A supplied RepositoryIDs replaces the whole membership
// list, which is also how reordering is expressed.
type UpdateRepositorySetRequest struct {
	Name          *string                     `json:"name"`
	Description   *string                     `json:"description"`
	RepositoryIDs *[]string                   `json:"repository_ids"`
	Repositories  *[]RepositorySetMemberInput `json:"repositories"`
}

// CreateRepositorySet validates the whole request, then writes the set and its
// membership together. Nothing is written when any check fails.
func (s *Service) CreateRepositorySet(
	ctx context.Context,
	req *CreateRepositorySetRequest,
) (*models.RepositorySet, error) {
	if err := s.authorizeWorkspaceID(ctx, req.WorkspaceID); err != nil {
		return nil, err
	}
	name, err := validateRepositorySetName(req.Name)
	if err != nil {
		return nil, err
	}
	if err := s.assertRepositorySetNameFree(ctx, req.WorkspaceID, name, ""); err != nil {
		return nil, err
	}
	items, err := repositorySetItemsFromCreateRequest(req)
	if err != nil {
		return nil, err
	}
	if err := s.validateRepositorySetMembers(ctx, req.WorkspaceID, items); err != nil {
		return nil, err
	}
	set := &models.RepositorySet{
		WorkspaceID: req.WorkspaceID,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Items:       items,
	}
	if err := s.repositorySets.CreateRepositorySet(ctx, set); err != nil {
		return nil, err
	}
	s.publishRepositorySetEvent(ctx, events.RepositorySetCreated, set)
	return set, nil
}

// GetRepositorySet loads one set, authorizing against the workspace that owns it.
func (s *Service) GetRepositorySet(ctx context.Context, id string) (*models.RepositorySet, error) {
	set, err := s.repositorySets.GetRepositorySet(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspaceID(ctx, set.WorkspaceID); err != nil {
		// A set in a workspace the caller cannot see is indistinguishable from
		// one that does not exist.
		return nil, repoerrors.ErrRepositorySetNotFound
	}
	return set, nil
}

// ListRepositorySets returns a workspace's sets with membership resolved.
func (s *Service) ListRepositorySets(ctx context.Context, workspaceID string) ([]*models.RepositorySet, error) {
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.repositorySets.ListRepositorySets(ctx, workspaceID)
}

// UpdateRepositorySet applies name, description, and membership changes
// together. It validates everything before the first write, so a rejected
// request leaves the set exactly as it was.
func (s *Service) UpdateRepositorySet(
	ctx context.Context,
	id string,
	req *UpdateRepositorySetRequest,
) (*models.RepositorySet, error) {
	set, err := s.GetRepositorySet(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.prepareRepositorySetUpdate(ctx, set, req); err != nil {
		return nil, err
	}
	// One call, one transaction: a rename that lands while the membership
	// replacement fails would leave the set renamed but still holding the old
	// repositories, with this method reporting failure and publishing nothing.
	var repositoryItems *[]models.RepositorySetItem
	if req.Repositories != nil || req.RepositoryIDs != nil {
		repositoryItems = &set.Items
	}
	if err := s.repositorySets.UpdateRepositorySet(ctx, set, repositoryItems); err != nil {
		return nil, err
	}
	updated, err := s.repositorySets.GetRepositorySet(ctx, set.ID)
	if err != nil {
		return nil, err
	}
	s.publishRepositorySetEvent(ctx, events.RepositorySetUpdated, updated)
	return updated, nil
}

// prepareRepositorySetUpdate validates the patch and folds the accepted values
// into set, leaving the caller to persist them.
func (s *Service) prepareRepositorySetUpdate(
	ctx context.Context,
	set *models.RepositorySet,
	req *UpdateRepositorySetRequest,
) error {
	if req.Name != nil {
		name, err := validateRepositorySetName(*req.Name)
		if err != nil {
			return err
		}
		if err := s.assertRepositorySetNameFree(ctx, set.WorkspaceID, name, set.ID); err != nil {
			return err
		}
		set.Name = name
	}
	if req.Description != nil {
		set.Description = strings.TrimSpace(*req.Description)
	}
	if req.Repositories != nil && req.RepositoryIDs != nil {
		return fmt.Errorf("%w: repositories and repository_ids cannot both be provided", ErrInvalidRepositorySet)
	}
	if req.Repositories != nil {
		items := repositorySetItemsForInputs(*req.Repositories)
		if err := s.validateRepositorySetMembers(ctx, set.WorkspaceID, items); err != nil {
			return err
		}
		set.Items = items
	}
	if req.RepositoryIDs != nil {
		items := repositorySetItemsFor(*req.RepositoryIDs)
		if err := s.validateRepositorySetMembers(ctx, set.WorkspaceID, items); err != nil {
			return err
		}
		set.Items = items
	}
	return nil
}

// DeleteRepositorySet removes a set and its membership. Repositories are never
// touched.
func (s *Service) DeleteRepositorySet(ctx context.Context, id string) error {
	set, err := s.GetRepositorySet(ctx, id)
	if err != nil {
		return err
	}
	deleted, err := s.repositorySets.DeleteRepositorySet(ctx, id)
	if err != nil {
		return err
	}
	if !deleted {
		return repoerrors.ErrRepositorySetNotFound
	}
	s.publishRepositorySetEvent(ctx, events.RepositorySetDeleted, set)
	return nil
}

// repositorySetIDsHolding reports which sets currently hold a repository. A
// lookup failure is logged and treated as "none": it must not block the
// repository deletion the caller is performing.
func (s *Service) repositorySetIDsHolding(ctx context.Context, repositoryID string) []string {
	if s.repositorySets == nil {
		return nil
	}
	ids, err := s.repositorySets.ListRepositorySetIDsByRepository(ctx, repositoryID)
	if err != nil {
		s.logger.Warn("failed to list repository sets holding repository",
			zap.String("repository_id", repositoryID), zap.Error(err))
		return nil
	}
	return ids
}

// publishRepositorySetsAfterMembershipChange re-reads each affected set and
// publishes its post-change shape, so clients that only react to
// repository_set.* events converge without a reload.
func (s *Service) publishRepositorySetsAfterMembershipChange(ctx context.Context, setIDs []string) {
	for _, setID := range setIDs {
		set, err := s.repositorySets.GetRepositorySet(ctx, setID)
		if err != nil {
			// A set deleted concurrently has its own deleted event; nothing to say.
			continue
		}
		s.publishRepositorySetEvent(ctx, events.RepositorySetUpdated, set)
	}
}

// validateRepositorySetName trims and bounds a set name.
func validateRepositorySetName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("%w: name is required", ErrInvalidRepositorySet)
	}
	// Count runes, not bytes: a 100-character name of non-ASCII text is still a
	// 100-character name to the user typing it.
	if len([]rune(name)) > repositorySetNameMaxLength {
		return "", fmt.Errorf("%w: name must be at most %d characters",
			ErrInvalidRepositorySet, repositorySetNameMaxLength)
	}
	return name, nil
}

// assertRepositorySetNameFree rejects a name already used in the workspace,
// naming the holder so the client can point at it. exceptID lets a set keep its
// own name across an update. The database's unique index on
// (workspace_id, LOWER(name)) is the backstop for two concurrent creates racing
// past this check, including two that differ only in case.
func (s *Service) assertRepositorySetNameFree(
	ctx context.Context,
	workspaceID string,
	name string,
	exceptID string,
) error {
	existing, err := s.repositorySets.GetRepositorySetByName(ctx, workspaceID, name)
	if err != nil {
		return err
	}
	if existing == nil || existing.ID == exceptID {
		return nil
	}
	return fmt.Errorf("%w: %q already uses this name", ErrRepositorySetNameConflict, existing.Name)
}

// validateRepositorySetMembers requires at least one member, no repeats, and
// every id resolving to a live repository in the set's workspace.
func (s *Service) validateRepositorySetMembers(
	ctx context.Context,
	workspaceID string,
	items []models.RepositorySetItem,
) error {
	if len(items) == 0 {
		return fmt.Errorf("%w: at least one repository is required", ErrInvalidRepositorySet)
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		id := item.RepositoryID
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("%w: repository ids must not be blank", ErrInvalidRepositorySet)
		}
		if _, repeated := seen[id]; repeated {
			return fmt.Errorf("%w: repository %q listed more than once", ErrInvalidRepositorySet, id)
		}
		seen[id] = struct{}{}
		if item.BaseBranch != "" && !securityutil.IsValidBaseBranchRef(item.BaseBranch) {
			return fmt.Errorf("%w: unsafe base branch for repository %q", ErrInvalidRepositorySet, id)
		}
	}
	// One workspace listing rather than a lookup per id, and it already excludes
	// soft-deleted rows and other workspaces.
	live, err := s.repoEntities.ListRepositories(ctx, workspaceID)
	if err != nil {
		return err
	}
	available := make(map[string]struct{}, len(live))
	for _, repository := range live {
		if repository != nil {
			available[repository.ID] = struct{}{}
		}
	}
	unknown := make([]string, 0)
	for _, item := range items {
		if _, ok := available[item.RepositoryID]; !ok {
			unknown = append(unknown, item.RepositoryID)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%w: %s", ErrUnknownRepositorySetMembers, strings.Join(unknown, ", "))
	}
	return nil
}

func repositorySetItemsFromCreateRequest(req *CreateRepositorySetRequest) ([]models.RepositorySetItem, error) {
	if req.Repositories != nil && req.RepositoryIDs != nil {
		return nil, fmt.Errorf("%w: repositories and repository_ids cannot both be provided", ErrInvalidRepositorySet)
	}
	if len(req.Repositories) > 0 {
		return repositorySetItemsForInputs(req.Repositories), nil
	}
	return repositorySetItemsFor(req.RepositoryIDs), nil
}

func repositorySetItemsForInputs(inputs []RepositorySetMemberInput) []models.RepositorySetItem {
	items := make([]models.RepositorySetItem, 0, len(inputs))
	for _, input := range inputs {
		items = append(items, models.RepositorySetItem{
			RepositoryID: input.RepositoryID,
			BaseBranch:   input.BaseBranch,
		})
	}
	return items
}

// repositorySetItemsFor turns an ordered id list into membership models. The
// store assigns positions from slice order.
func repositorySetItemsFor(repositoryIDs []string) []models.RepositorySetItem {
	items := make([]models.RepositorySetItem, 0, len(repositoryIDs))
	for _, id := range repositoryIDs {
		items = append(items, models.RepositorySetItem{RepositoryID: id})
	}
	return items
}

// publishRepositorySetEvent carries id and workspace_id so the gateway's
// existing workspace routing delivers the event without a new routing branch.
func (s *Service) publishRepositorySetEvent(
	ctx context.Context,
	eventType string,
	set *models.RepositorySet,
) {
	if s.eventBus == nil || set == nil {
		return
	}
	repositories := make([]map[string]interface{}, 0, len(set.Items))
	for _, item := range set.Items {
		repositories = append(repositories, map[string]interface{}{
			"repository_id": item.RepositoryID,
			"position":      item.Position,
			"base_branch":   item.BaseBranch,
		})
	}
	data := map[string]interface{}{
		"id":           set.ID,
		"workspace_id": set.WorkspaceID,
		"name":         set.Name,
		"description":  set.Description,
		"repositories": repositories,
		"created_at":   set.CreatedAt.Format(time.RFC3339),
		"updated_at":   set.UpdatedAt.Format(time.RFC3339),
	}
	event := bus.NewEvent(eventType, "task-service", data)
	if err := s.eventBus.Publish(ctx, eventType, event); err != nil {
		s.logger.Error("failed to publish repository set event",
			zap.String("event_type", eventType),
			zap.String("repository_set_id", set.ID),
			zap.Error(err))
	}
}
