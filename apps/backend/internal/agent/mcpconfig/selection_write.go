package mcpconfig

import (
	"context"
	"errors"
)

var ErrMCPSelectionConflict = errors.New("MCP selections changed")

// SelectionCondition distinguishes an omitted precondition from known-empty membership.
// Sessions use their desired revision; other scopes compare direct membership.
type SelectionCondition struct {
	// ValidateAdditions rechecks external owner eligibility only when adding IDs.
	ValidateAdditions     func(context.Context) error
	ExpectedDefinitionIDs *[]string
	ExpectedRevision      *int64
	ExcludedDefinitionIDs *[]string
	ExclusionContext      *ResolutionContext
}

func (c SelectionCondition) Validate(scope SelectionScope) error {
	if scope == SelectionScopeTaskSession {
		if c.ExpectedDefinitionIDs != nil {
			return ErrMCPInvalidSelection
		}
	} else if c.ExpectedRevision != nil {
		return ErrMCPInvalidSelection
	}
	if c.ExcludedDefinitionIDs != nil && (scope != SelectionScopeTaskSession || c.ExpectedRevision == nil) {
		return ErrMCPInvalidSelection
	}
	if c.ExpectedRevision != nil && *c.ExpectedRevision < 0 {
		return ErrMCPInvalidSelection
	}
	return nil
}

// SelectionSnapshot belongs to one read or committed replacement. State never
// comes from a subsequent reread of another writer's selection.
type SelectionSnapshot struct {
	DefinitionIDs         []string
	ExcludedDefinitionIDs []string
	State                 *SessionMCPSelectionState
}

type ConditionalSelectionRepository interface {
	ReadMCPSelectionSnapshot(context.Context, SelectionOrigin) (SelectionSnapshot, error)
	ReplaceMCPSelectionsConditional(context.Context, SelectionOrigin, []string, SelectionCondition) (SelectionSnapshot, error)
}

func (s *SelectionService) SetAdditionValidator(validator SelectionOwnerValidator) {
	s.additionValidator = validator
}

func (s *SelectionService) ReadSnapshot(ctx context.Context, origin SelectionOrigin) (SelectionSnapshot, error) {
	if err := s.validateContext(ctx, origin.Scope, origin.WorkspaceID, origin.OwnerID); err != nil {
		return SelectionSnapshot{}, err
	}
	if repo, ok := s.repo.(ConditionalSelectionRepository); ok {
		snapshot, err := repo.ReadMCPSelectionSnapshot(ctx, origin)
		if err != nil {
			return snapshot, err
		}
		return s.safeSelectionSnapshot(ctx, origin, snapshot), nil
	}
	ids, err := s.List(ctx, origin.Scope, origin.WorkspaceID, origin.OwnerID)
	snapshot := SelectionSnapshot{DefinitionIDs: ids}
	if err == nil && origin.Scope == SelectionScopeTaskSession && s.stateRepo != nil {
		state, stateErr := s.stateRepo.GetMCPSelectionState(ctx, origin.OwnerID)
		if stateErr == nil {
			snapshot.State = &state
		} else if !errors.Is(stateErr, ErrMCPSelectionStateNotFound) {
			return snapshot, stateErr
		}
	}
	return s.safeSelectionSnapshot(ctx, origin, snapshot), err
}

func (s *SelectionService) ReplaceConditional(ctx context.Context, origin SelectionOrigin, ids []string, condition SelectionCondition) (SelectionSnapshot, error) {
	if s.additionValidator != nil {
		condition.ValidateAdditions = func(ctx context.Context) error {
			if err := s.additionValidator(ctx, origin.Scope, origin.WorkspaceID, origin.OwnerID); err != nil {
				return errors.Join(ErrMCPSelectionOwnerAccess, err)
			}
			return nil
		}
	}
	return s.replaceConditional(ctx, origin, ids, condition)
}

func (s *SelectionService) replaceConditional(ctx context.Context, origin SelectionOrigin, ids []string, condition SelectionCondition) (SelectionSnapshot, error) {
	if err := condition.Validate(origin.Scope); err != nil {
		return SelectionSnapshot{}, err
	}
	if err := s.validateContext(ctx, origin.Scope, origin.WorkspaceID, origin.OwnerID); err != nil {
		return SelectionSnapshot{}, err
	}
	if err := s.prepareExclusionCondition(ctx, origin, &condition); err != nil {
		return SelectionSnapshot{}, err
	}
	if repo, ok := s.repo.(ConditionalSelectionRepository); ok {
		snapshot, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, ids, condition)
		if err != nil {
			return snapshot, err
		}
		if origin.Scope == SelectionScopeTaskSession {
			s.notifySessionMCPChange(ctx, origin.OwnerID)
		}
		return s.safeSelectionSnapshot(ctx, origin, snapshot), nil
	}
	// Adapters without transactional admission may serve legacy callers only.
	if condition.ExpectedDefinitionIDs != nil || condition.ExpectedRevision != nil || condition.ExcludedDefinitionIDs != nil {
		return SelectionSnapshot{}, ErrMCPInvalidSelection
	}
	if err := s.Replace(ctx, origin.Scope, origin.WorkspaceID, origin.OwnerID, ids); err != nil {
		return SelectionSnapshot{}, err
	}
	return s.ReadSnapshot(ctx, origin)
}

func (s *SelectionService) safeSelectionSnapshot(ctx context.Context, origin SelectionOrigin, snapshot SelectionSnapshot) SelectionSnapshot {
	if snapshot.DefinitionIDs == nil {
		snapshot.DefinitionIDs = []string{}
	}
	if snapshot.State != nil {
		state := *snapshot.State
		state.Comparison = s.sessionSetupComparison(ctx, origin.WorkspaceID, origin.OwnerID, state)
		state.AcceptedSetup = nil
		snapshot.State = &state
	}
	return snapshot
}

func (s *SelectionService) prepareExclusionCondition(ctx context.Context, origin SelectionOrigin, condition *SelectionCondition) error {
	if condition.ExcludedDefinitionIDs == nil {
		return nil
	}
	if s.resolutionContext == nil {
		return ErrMCPInvalidSelection
	}
	rc, err := s.resolutionContext(ctx, origin.WorkspaceID, origin.OwnerID)
	if err != nil || rc.WorkspaceID != origin.WorkspaceID || rc.SessionID != origin.OwnerID {
		return ErrMCPInvalidSelection
	}
	condition.ExclusionContext = &rc
	return nil
}
