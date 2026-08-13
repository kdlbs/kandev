package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

// ErrInvalidDisposition signals that a caller-supplied disposition body was
// rejected: an unknown enum value, a superseded_by_url paired with a
// non-superseded disposition, or a superseded_by_url that resolves to the
// association's own PR.
var ErrInvalidDisposition = errors.New("invalid task PR disposition")

// SetTaskPRDisposition records (or clears) a human-supplied closure reason
// for a task-PR association. Order of operations mirrors DetachTaskPR: the
// association is looked up and authorized before any validation, so a
// superseded_by_url self-reference check has the association's own identity
// available and authorization matches every other task-PR mutation.
//
// A nil disposition — whether the JSON key was absent or explicitly null —
// means clear: all three disposition columns are reset to NULL in one
// statement (AC-22). The endpoint accepts the write regardless of the
// association's state (AC-29b) or detached_at (AC-27): a detached
// association is exactly a PR someone walked away from, and a user may
// record intent on a PR they are about to close.
func (s *Service) SetTaskPRDisposition(
	ctx context.Context, workspaceID, associationID string, disposition, supersededByURL *string,
) (*TaskPR, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(associationID) == "" {
		return nil, ErrTaskPRNotFound
	}
	tp, err := s.store.GetTaskPRByID(ctx, associationID)
	if err != nil {
		return nil, err
	}
	if tp == nil || (tp.WorkspaceID != "" && tp.WorkspaceID != workspaceID) {
		return nil, ErrTaskPRNotFound
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return nil, err
	}

	normalizedURL, err := normalizeTaskPRDisposition(tp, disposition, supersededByURL)
	if err != nil {
		return nil, err
	}

	if dispositionUnchanged(tp, disposition, normalizedURL) {
		return tp, nil
	}

	var recordedAt *time.Time
	action := "clear"
	if disposition != nil {
		now := time.Now().UTC()
		recordedAt = &now
		action = "set"
	}
	if err := s.store.UpdateTaskPRDisposition(ctx, associationID, disposition, normalizedURL, recordedAt); err != nil {
		return nil, err
	}
	incTaskPROutcomeDisposition(action)

	updated, err := s.store.GetTaskPRByID(ctx, associationID)
	if err != nil {
		return nil, err
	}
	if s.eventBus != nil {
		event := bus.NewEvent(events.GitHubTaskPRUpdated, "github", updated)
		if err := s.eventBus.Publish(ctx, events.GitHubTaskPRUpdated, event); err != nil {
			s.logger.Debug("failed to publish task PR updated event", zap.Error(err))
		}
	}
	return updated, nil
}

// normalizeTaskPRDisposition validates a disposition write body against tp
// and returns the trimmed supersededByURL (nil when absent). An empty
// supersededByURL string is treated as absent, not as an invalid URL.
func normalizeTaskPRDisposition(tp *TaskPR, disposition, supersededByURL *string) (*string, error) {
	var normalizedURL *string
	if supersededByURL != nil {
		trimmed := strings.TrimSpace(*supersededByURL)
		if trimmed != "" {
			normalizedURL = &trimmed
		}
	}

	if disposition != nil && !validTaskPRDisposition(*disposition) {
		return nil, ErrInvalidDisposition
	}
	if normalizedURL != nil && (disposition == nil || *disposition != TaskPRDispositionSuperseded) {
		return nil, ErrInvalidDisposition
	}
	if normalizedURL == nil {
		return nil, nil
	}

	owner, repo, number, err := parsePRURL(*normalizedURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPRURL, err)
	}
	if strings.EqualFold(owner, tp.Owner) && strings.EqualFold(repo, tp.Repo) && number == tp.PRNumber {
		return nil, ErrInvalidDisposition
	}
	return normalizedURL, nil
}

// dispositionUnchanged reports whether the desired write is identical to
// what is already stored, so SetTaskPRDisposition can write nothing,
// publish nothing, and leave disposition_recorded_at byte-identical on a
// repeated PATCH (AC-29).
func dispositionUnchanged(tp *TaskPR, disposition, supersededByURL *string) bool {
	return stringPtrEqual(tp.Disposition, disposition) && stringPtrEqual(tp.DispositionSupersededByURL, supersededByURL)
}
