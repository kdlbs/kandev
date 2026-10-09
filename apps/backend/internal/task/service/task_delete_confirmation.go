package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

var (
	ErrTaskDeleteConfirmationRequired = errors.New("task deletion confirmation is required")
	ErrTaskDeleteConfirmationExpired  = errors.New("task deletion confirmation expired")
	ErrTaskDeleteConfirmationStale    = errors.New("task deletion preview is stale")
	ErrTaskDeleteConfirmationReplay   = errors.New("task deletion confirmation was already used")
	ErrTaskDeleteConfirmationMismatch = errors.New("task deletion confirmation does not match this request")
	ErrTaskDeleteConfirmationIdentity = errors.New("task deletion confirmation requires an authenticated user")
)

const taskDeleteConfirmationTTL = 5 * time.Minute

func taskDeletePreviewUserID(ctx context.Context) (string, bool) {
	identity, ok := authn.IdentityFromContext(ctx)
	return identity.UserID, ok && identity.UserID != ""
}

type taskDeletePreview struct {
	ownerUserID            string
	rootIDs                map[string]struct{}
	rootDigests            map[string]string
	cascade                bool
	discardWorktreeChanges bool
	expiresAt              time.Time
	consumedRoots          map[string]struct{}
}

type taskDeletePreviewRow struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspace_id"`
	ParentID       string `json:"parent_id"`
	UpdatedAt      string `json:"updated_at"`
	ArchivedAt     string `json:"archived_at"`
	Origin         string `json:"origin"`
	ExternalID     string `json:"external_id"`
	PluginSource   string `json:"plugin_source"`
	WorkflowID     string `json:"workflow_id"`
	WorkflowStepID string `json:"workflow_step_id"`
}

// WithTaskDeleteConfirmation consumes a native preview for one root, then
// runs the existing delete lifecycle once. The ticket is bound to its Human,
// options, selected roots, current affected-task set, and row versions.
//
//nolint:cyclop // The cleanup receipt is verified immediately before the task deletion effect.
func (s *Service) WithTaskDeleteConfirmation(
	ctx context.Context,
	confirmationID, rootID string,
	cascade, discardWorktreeChanges bool,
	deleteFn func() error,
) error {
	if err := s.authorizeTaskScope(ctx, rootID, authz.ScopeTaskWrite); err != nil {
		return err
	}
	if task, err := s.tasks.GetTask(ctx, rootID); err != nil {
		return err
	} else if task == nil {
		return repoerrors.ErrTaskNotFound
	}
	if confirmationID == "" || rootID == "" || deleteFn == nil {
		return ErrTaskDeleteConfirmationRequired
	}
	userID, ok := taskDeletePreviewUserID(ctx)
	if !ok {
		return ErrTaskDeleteConfirmationIdentity
	}
	s.taskDeletePreviewMu.Lock()
	s.pruneTaskDeletePreviewsLocked(time.Now().UTC())
	preview, ok := s.taskDeletePreviews[confirmationID]
	if !ok {
		s.taskDeletePreviewMu.Unlock()
		return ErrTaskDeleteConfirmationExpired
	}
	if preview.ownerUserID != userID || preview.cascade != cascade || preview.discardWorktreeChanges != discardWorktreeChanges {
		s.taskDeletePreviewMu.Unlock()
		return ErrTaskDeleteConfirmationMismatch
	}
	if _, selected := preview.rootIDs[rootID]; !selected {
		s.taskDeletePreviewMu.Unlock()
		return ErrTaskDeleteConfirmationMismatch
	}
	if _, consumed := preview.consumedRoots[rootID]; consumed {
		s.taskDeletePreviewMu.Unlock()
		return ErrTaskDeleteConfirmationReplay
	}
	digest, err := s.taskDeletePreviewDigest(ctx, rootID, cascade)
	if err != nil {
		s.taskDeletePreviewMu.Unlock()
		return fmt.Errorf("validate task deletion preview: %w", err)
	}
	if digest != preview.rootDigests[rootID] {
		delete(s.taskDeletePreviews, confirmationID)
		s.taskDeletePreviewMu.Unlock()
		return ErrTaskDeleteConfirmationStale
	}
	preview.consumedRoots[rootID] = struct{}{}
	s.taskDeletePreviewMu.Unlock()
	return deleteFn()
}

func (s *Service) issueTaskDeletePreview(ctx context.Context, roots []string, cascade, discardWorktreeChanges bool) (string, error) {
	userID, ok := taskDeletePreviewUserID(ctx)
	if !ok {
		return "", ErrTaskDeleteConfirmationIdentity
	}
	rootIDs := make(map[string]struct{}, len(roots))
	rootDigests := make(map[string]string, len(roots))
	for _, rootID := range roots {
		digest, err := s.taskDeletePreviewDigest(ctx, rootID, cascade)
		if err != nil {
			return "", fmt.Errorf("create task deletion preview: %w", err)
		}
		rootIDs[rootID] = struct{}{}
		rootDigests[rootID] = digest
	}
	confirmationID := uuid.NewString()
	preview := taskDeletePreview{
		ownerUserID: userID, rootIDs: rootIDs, rootDigests: rootDigests,
		cascade: cascade, discardWorktreeChanges: discardWorktreeChanges,
		expiresAt: time.Now().UTC().Add(taskDeleteConfirmationTTL), consumedRoots: make(map[string]struct{}),
	}
	s.taskDeletePreviewMu.Lock()
	s.pruneTaskDeletePreviewsLocked(time.Now().UTC())
	if s.taskDeletePreviews == nil {
		s.taskDeletePreviews = make(map[string]taskDeletePreview)
	}
	s.taskDeletePreviews[confirmationID] = preview
	s.taskDeletePreviewMu.Unlock()
	return confirmationID, nil
}

func (s *Service) taskDeletePreviewDigest(ctx context.Context, rootID string, cascade bool) (string, error) {
	ids, err := s.resolveTaskDeletePreflightTargets(ctx, []string{rootID}, cascade)
	if err != nil {
		return "", err
	}
	if !cascade {
		children, err := s.tasks.ListChildrenIncludingArchived(ctx, rootID)
		if err != nil {
			return "", fmt.Errorf("list affected direct children: %w", err)
		}
		for _, child := range children {
			if child != nil && child.ID != "" {
				ids = append(ids, child.ID)
			}
		}
	}
	ids = normalizeTaskDeletePreflightIDs(ids)
	sort.Strings(ids)
	rows := make([]taskDeletePreviewRow, 0, len(ids))
	for _, id := range ids {
		task, err := s.tasks.GetTask(ctx, id)
		if err != nil {
			return "", err
		}
		if task == nil || task.ID != id {
			return "", ErrTaskDeleteConfirmationStale
		}
		archivedAt := ""
		if task.ArchivedAt != nil {
			archivedAt = task.ArchivedAt.UTC().Format(time.RFC3339Nano)
		}
		pluginSource, _ := task.Metadata["source"].(string)
		rows = append(rows, taskDeletePreviewRow{
			ID: task.ID, WorkspaceID: task.WorkspaceID, ParentID: task.ParentID,
			UpdatedAt: task.UpdatedAt.UTC().Format(time.RFC3339Nano), ArchivedAt: archivedAt,
			Origin: task.Origin, ExternalID: task.ExternalID, PluginSource: pluginSource,
			WorkflowID: task.WorkflowID, WorkflowStepID: task.WorkflowStepID,
		})
	}
	encoded, err := json.Marshal(struct {
		RootID  string                 `json:"root_id"`
		Cascade bool                   `json:"cascade"`
		Rows    []taskDeletePreviewRow `json:"rows"`
	}{RootID: rootID, Cascade: cascade, Rows: rows})
	if err != nil {
		return "", fmt.Errorf("encode task deletion preview: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (s *Service) pruneTaskDeletePreviewsLocked(now time.Time) {
	for id, preview := range s.taskDeletePreviews {
		if !now.Before(preview.expiresAt) {
			delete(s.taskDeletePreviews, id)
		}
	}
}
