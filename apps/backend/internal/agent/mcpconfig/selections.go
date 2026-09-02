package mcpconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// SelectionScope identifies the owner of a set of MCP definition references.
// Profile selections always carry a workspace because global profiles are
// allowed to choose different servers in different workspaces.
type SelectionScope string

const (
	SelectionScopeProfile     SelectionScope = "profile"
	SelectionScopeRepository  SelectionScope = "repository"
	SelectionScopeTask        SelectionScope = "task"
	SelectionScopeTaskSession SelectionScope = "task_session"
)

var (
	ErrMCPInvalidSelection           = errors.New("invalid MCP selection")
	ErrMCPSelectionWorkspaceMismatch = errors.New("MCP selection does not belong to the workspace")
	ErrMCPDefinitionDisabled         = errors.New("MCP server definition is disabled")
	ErrMCPSelectionOwnerAccess       = errors.New("MCP selection owner access denied")
	ErrMCPSelectionStateNotFound     = errors.New("MCP selection state not found")
)

// SessionMCPApplyState describes the durable relationship between a desired
// session selection and the last provider configuration that was accepted.
type SessionMCPApplyState string

const (
	SessionMCPApplyStateApplied         SessionMCPApplyState = "applied"
	SessionMCPApplyStatePendingIdle     SessionMCPApplyState = "pending_idle"
	SessionMCPApplyStateDeferredRestart SessionMCPApplyState = "deferred_restart"
	SessionMCPApplyStateFailed          SessionMCPApplyState = "failed"
)

// SessionMCPSelectionState is safe to expose to the UI. It contains only
// revisions, sanitized failure text, and the backend-owned attachment attempt.
type SessionMCPSelectionState struct {
	DesiredRevision     int64                `json:"desired_revision"`
	AppliedRevision     int64                `json:"applied_revision"`
	ApplyState          SessionMCPApplyState `json:"apply_state"`
	FailureCode         string               `json:"failure_code,omitempty"`
	FailureSummary      string               `json:"failure_summary,omitempty"`
	AttachmentAttemptID string               `json:"attachment_attempt_id,omitempty"`
}

// SessionMCPSelectionStateRepository persists desired/applied session state.
type SessionMCPSelectionStateRepository interface {
	GetMCPSelectionState(context.Context, string) (SessionMCPSelectionState, error)
	SaveMCPSelectionState(context.Context, string, SessionMCPSelectionState) error
}

// SelectionImpact counts the durable scopes that reference one definition.
// Associations remain present when a definition is disabled so re-enabling it
// restores the user's choices.
type SelectionImpact struct {
	Profile     int `json:"profile"`
	Repository  int `json:"repository"`
	Task        int `json:"task"`
	TaskSession int `json:"task_session"`
}

func (i SelectionImpact) Total() int {
	return i.Profile + i.Repository + i.Task + i.TaskSession
}

// MCPSelectionImpactError is returned by a guarded delete before any rows are
// changed. The caller can show the affected scope counts and retry with an
// explicit confirmation.
type MCPSelectionImpactError struct {
	Impact SelectionImpact
}

func (e *MCPSelectionImpactError) Error() string {
	return fmt.Sprintf("%s: %d selection(s)", ErrMCPDefinitionSelected, e.Impact.Total())
}

func (e *MCPSelectionImpactError) Unwrap() error { return ErrMCPDefinitionSelected }

// SelectionRepository stores only definition IDs. It does not accept raw MCP
// configuration or secret values.
type SelectionRepository interface {
	ListMCPSelections(context.Context, SelectionScope, string, string) ([]string, error)
	ReplaceMCPSelections(context.Context, SelectionScope, string, string, []string) error
	SelectionImpact(context.Context, string, string) (SelectionImpact, error)
	DeleteMCPSelectionsForDefinition(context.Context, string, string) error
}

// SelectionOwnerValidator verifies that ownerID belongs to the requested
// workspace. The task system supplies this callback because those owner rows
// live in the task database, not the settings database.
type SelectionOwnerValidator func(context.Context, SelectionScope, string, string) error

// SelectionService validates workspace ownership and atomically replaces
// scope selections. It deliberately has no resolver or runtime side effects.
type SelectionService struct {
	repo           SelectionRepository
	catalog        CatalogRepository
	authorizer     WorkspaceAuthorizer
	ownerValidator SelectionOwnerValidator
	stateRepo      SessionMCPSelectionStateRepository
	stateNotifier  func(context.Context, string)
	stateMu        sync.Mutex
}

func NewSelectionService(repo SelectionRepository, catalog CatalogRepository) *SelectionService {
	return &SelectionService{repo: repo, catalog: catalog}
}

// Repository returns the storage adapter for composition of catalog deletion
// and selection services. Callers still use SelectionService for validation.
func (s *SelectionService) Repository() SelectionRepository {
	if s == nil {
		return nil
	}
	return s.repo
}

func (s *SelectionService) SetWorkspaceAuthorizer(authorizer WorkspaceAuthorizer) {
	s.authorizer = authorizer
}

func (s *SelectionService) SetOwnerValidator(validator SelectionOwnerValidator) {
	s.ownerValidator = validator
}

// SetSessionMCPStateRepository enables durable desired/applied state for
// task-session selections.
func (s *SelectionService) SetSessionMCPStateRepository(repo SessionMCPSelectionStateRepository) {
	s.stateRepo = repo
}

// SetSessionMCPChangeNotifier schedules provider-side application after a
// task-session selection changes. The callback must not trust request payload
// identities; it receives only the authorized session ID.
func (s *SelectionService) SetSessionMCPChangeNotifier(notifier func(context.Context, string)) {
	s.stateNotifier = notifier
}

// SessionState returns the durable desired/applied state for a task session.
// It validates ownership before exposing state so a session ID cannot be used
// as a cross-workspace lookup key.
func (s *SelectionService) SessionState(
	ctx context.Context,
	workspaceID, sessionID string,
) (SessionMCPSelectionState, error) {
	if err := s.validateContext(ctx, SelectionScopeTaskSession, workspaceID, sessionID); err != nil {
		return SessionMCPSelectionState{}, err
	}
	if s.stateRepo == nil {
		return SessionMCPSelectionState{}, ErrMCPSelectionStateNotFound
	}
	return s.stateRepo.GetMCPSelectionState(ctx, sessionID)
}

func (s *SelectionService) List(ctx context.Context, scope SelectionScope, workspaceID, ownerID string) ([]string, error) {
	if err := s.validateContext(ctx, scope, workspaceID, ownerID); err != nil {
		return nil, err
	}
	return s.repo.ListMCPSelections(ctx, scope, workspaceID, ownerID)
}

func (s *SelectionService) Replace(ctx context.Context, scope SelectionScope, workspaceID, ownerID string, definitionIDs []string) error {
	if err := s.validateContext(ctx, scope, workspaceID, ownerID); err != nil {
		return err
	}
	if s.catalog == nil {
		return errors.New("MCP catalog is unavailable")
	}
	ids := uniqueSelectionIDs(definitionIDs)
	for _, definitionID := range ids {
		definition, err := s.catalog.GetMCPServerDefinition(ctx, workspaceID, definitionID)
		if errors.Is(err, ErrMCPServerDefinitionNotFound) || definition == nil {
			return fmt.Errorf("%w: %s", ErrMCPSelectionWorkspaceMismatch, definitionID)
		}
		if err != nil {
			return err
		}
		if definition.WorkspaceID != workspaceID {
			return fmt.Errorf("%w: %s", ErrMCPSelectionWorkspaceMismatch, definitionID)
		}
		if !definition.Enabled {
			return fmt.Errorf("%w: %s", ErrMCPDefinitionDisabled, definitionID)
		}
	}
	if err := s.repo.ReplaceMCPSelections(ctx, scope, workspaceID, ownerID, ids); err != nil {
		return err
	}
	if scope == SelectionScopeTaskSession {
		if err := s.recordSessionMCPChange(ctx, ownerID); err != nil {
			return err
		}
	}
	return nil
}

func (s *SelectionService) recordSessionMCPChange(ctx context.Context, sessionID string) error {
	if s.stateRepo == nil {
		return nil
	}
	s.stateMu.Lock()
	state, err := s.stateRepo.GetMCPSelectionState(ctx, sessionID)
	if err != nil && !errors.Is(err, ErrMCPSelectionStateNotFound) {
		s.stateMu.Unlock()
		return err
	}
	state.DesiredRevision++
	state.ApplyState = SessionMCPApplyStatePendingIdle
	state.FailureCode = ""
	state.FailureSummary = ""
	if err := s.stateRepo.SaveMCPSelectionState(ctx, sessionID, state); err != nil {
		s.stateMu.Unlock()
		return err
	}
	s.stateMu.Unlock()
	if s.stateNotifier != nil {
		s.stateNotifier(context.WithoutCancel(ctx), sessionID)
	}
	return nil
}

func (s *SelectionService) SelectionImpact(ctx context.Context, workspaceID, definitionID string) (SelectionImpact, error) {
	if err := s.authorize(ctx, workspaceID); err != nil {
		return SelectionImpact{}, err
	}
	if strings.TrimSpace(definitionID) == "" {
		return SelectionImpact{}, ErrMCPInvalidSelection
	}
	return s.repo.SelectionImpact(ctx, workspaceID, definitionID)
}

func (s *SelectionService) DeleteDefinitionSelections(ctx context.Context, workspaceID, definitionID string) error {
	if err := s.authorize(ctx, workspaceID); err != nil {
		return err
	}
	if strings.TrimSpace(definitionID) == "" {
		return ErrMCPInvalidSelection
	}
	return s.repo.DeleteMCPSelectionsForDefinition(ctx, workspaceID, definitionID)
}

func (s *SelectionService) validateContext(ctx context.Context, scope SelectionScope, workspaceID, ownerID string) error {
	if !validSelectionScope(scope) || strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(ownerID) == "" {
		return ErrMCPInvalidSelection
	}
	if err := s.authorize(ctx, workspaceID); err != nil {
		return err
	}
	if s.ownerValidator == nil {
		return nil
	}
	if err := s.ownerValidator(ctx, scope, workspaceID, ownerID); err != nil {
		return fmt.Errorf("%w: %w", ErrMCPSelectionOwnerAccess, err)
	}
	return nil
}

func (s *SelectionService) authorize(ctx context.Context, workspaceID string) error {
	if s.authorizer == nil {
		return nil
	}
	if err := s.authorizer(ctx, workspaceID); err != nil {
		return fmt.Errorf("%w: %w", ErrMCPWorkspaceAccess, err)
	}
	return nil
}

func validSelectionScope(scope SelectionScope) bool {
	switch scope {
	case SelectionScopeProfile, SelectionScopeRepository, SelectionScopeTask, SelectionScopeTaskSession:
		return true
	default:
		return false
	}
}

func uniqueSelectionIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}
