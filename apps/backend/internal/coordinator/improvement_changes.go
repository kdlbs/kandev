package coordinator

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
)

// changeReadable authorizes a read of the coordinator's changes and resolves
// the coordinator inside the workspace.
func (s *Service) changeCoordinator(ctx context.Context, workspaceID, coordinatorID string, scope authz.Scope) (*Coordinator, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, scope); err != nil {
		return nil, err
	}
	if !s.phase3 {
		return nil, ErrChangeNotFound
	}
	c, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, readFailed("coordinator", err)
	}
	return c, nil
}

// ListPendingChanges returns the coordinator's pending changes, oldest first.
func (s *Service) ListPendingChanges(ctx context.Context, workspaceID, coordinatorID string) ([]*PendingChange, error) {
	c, err := s.changeCoordinator(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceRead)
	if err != nil {
		return nil, err
	}
	changes, err := s.store.ListPendingChanges(ctx, c.ID)
	if err != nil {
		return nil, readFailed("pending changes", err)
	}
	return changes, nil
}

// DiscardPendingChange settles a pending change discarded.
func (s *Service) DiscardPendingChange(ctx context.Context, workspaceID, coordinatorID, changeID string) (*PendingChange, error) {
	c, err := s.changeCoordinator(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceManage)
	if err != nil {
		return nil, err
	}
	change, err := s.store.DiscardPendingChange(ctx, c.ID, changeID, decidingUserID(ctx))
	if err != nil {
		if errors.Is(err, ErrChangeNotFound) || errors.As(err, new(*ChangeConflictError)) {
			return nil, err
		}
		return nil, readFailed("pending change", err)
	}
	countImprovement(improvementDiscarded)
	s.publishCoordinatorUpdated(ctx, workspaceID, coordinatorID)
	return change, nil
}

// ApplyPendingChange writes a pending change's context the way a manager's
// context edit does. Under the coordinator's write lock it re-validates the
// stored value, requires the stored context to still equal the change's base,
// runs the PATCH validation, writes conditionally on that base and settles the
// change, all in one transaction.
func (s *Service) ApplyPendingChange(ctx context.Context, workspaceID, coordinatorID, changeID string) (*Coordinator, error) {
	c, err := s.changeCoordinator(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceManage)
	if err != nil {
		return nil, err
	}
	decidedBy := decidingUserID(ctx)
	guard := &patchGuard{}
	guard.pre = func(ctx context.Context, exec coordinatorExec, row *coordinatorRow, patch *CoordinatorPatch) error {
		return s.prepareApply(ctx, exec, row, patch, guard, changeID)
	}
	guard.post = func(ctx context.Context, exec coordinatorExec) error {
		matched, err := s.store.settlePendingChangeTx(ctx, exec, c.ID, changeID, ChangeStatusApplied, decidedBy)
		if err != nil {
			return err
		}
		if matched {
			return nil
		}
		current, _, err := s.store.changeOn(ctx, exec, c.ID, changeID)
		if err != nil {
			return err
		}
		if current == nil {
			return ErrChangeNotFound
		}
		return &ChangeConflictError{Change: current}
	}
	result, err := s.store.patchCoordinatorGuarded(ctx, workspaceID, c.ID, CoordinatorPatch{}, s.patchValidator(workspaceID, CoordinatorPatch{}), guard)
	if err != nil {
		return nil, s.applyError(err)
	}
	countImprovement(improvementApplied)
	if result.ClearedConversationTaskID != nil && s.onConversationCleared != nil {
		s.onConversationCleared(ctx, c.ID, *result.ClearedConversationTaskID)
	}
	s.logger.Info("improvement applied", zap.String("coordinator_id", c.ID), zap.String("change_id", changeID))
	s.publishCoordinatorUpdated(ctx, workspaceID, c.ID)
	return result.Coordinator, nil
}

// prepareApply is the pre-write half of Apply: it runs on the locked row and
// completes the patch from the change it reads.
func (s *Service) prepareApply(ctx context.Context, exec coordinatorExec, row *coordinatorRow, patch *CoordinatorPatch, guard *patchGuard, changeID string) error {
	change, proposalStatus, err := s.store.changeOn(ctx, exec, row.ID, changeID)
	if err != nil {
		return readFailed("pending change", err)
	}
	if change == nil || proposalStatus != string(ProposalStatusApproved) {
		return ErrChangeNotFound
	}
	if change.Status != ChangeStatusPending {
		return &ChangeConflictError{Change: change}
	}
	next, err := ValidateContext(change.NewValue)
	if err != nil {
		return err
	}
	conflict := &ChangeConflictError{Change: change, Reason: ChangeReasonContextChanged}
	if row.Context != change.BaseValue {
		return conflict
	}
	patch.Context = &next
	guard.pinContext = &change.BaseValue
	guard.onPinMismatch = conflict
	return nil
}

// applyError counts a context conflict and leaves every other error as is.
func (s *Service) applyError(err error) error {
	var conflict *ChangeConflictError
	if errors.As(err, &conflict) && conflict.Reason == ChangeReasonContextChanged {
		countImprovement(improvementApplyConflict)
	}
	return err
}

func (h *Handlers) httpListPendingChanges(c *gin.Context) {
	changes, err := h.service.ListPendingChanges(c.Request.Context(), c.Param("id"), c.Param("cid"))
	if err != nil {
		h.respondReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"changes": changes})
}

func (h *Handlers) httpApplyPendingChange(c *gin.Context) {
	ctx := c.Request.Context()
	updated, err := h.service.ApplyPendingChange(ctx, c.Param("id"), c.Param("cid"), c.Param("chid"))
	if err != nil {
		h.respondReadError(c, err)
		return
	}
	dto, err := h.coordinatorDTO(ctx, updated)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto)
}

func (h *Handlers) httpDiscardPendingChange(c *gin.Context) {
	change, err := h.service.DiscardPendingChange(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("chid"))
	if err != nil {
		h.respondReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, change)
}
