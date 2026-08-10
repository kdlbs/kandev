package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/kandev/kandev/internal/task/models"
)

// ExternalIDMaxBytes is the maximum UTF-8 byte length of a normalized
// external_id (docs/specs/tasks/external-id-idempotency/spec.md,
// "Validation and normalization").
const ExternalIDMaxBytes = 255

// ErrExternalIDInvalid identifies a caller-supplied external_id that fails
// validation: a raw control character, or a normalized value over
// ExternalIDMaxBytes.
var ErrExternalIDInvalid = errors.New("invalid external_id")

// NormalizeExternalID applies the spec's validation/normalization rules to a
// raw caller-supplied external_id, in this exact order — the order is
// normative, since it decides which error a malformed value produces:
//
//  1. Reject ASCII control characters (U+0000-U+001F, U+007F) anywhere in the
//     raw value, before any trimming. This is what makes a trailing newline
//     (or a lone tab) an error rather than something the trim silently
//     erases into "absent".
//  2. Trim leading/trailing Unicode whitespace (the White_Space property).
//  3. Empty after trim -> treat as absent ("", nil error). Not an error.
//  4. The trimmed value's UTF-8 byte length MUST be <= ExternalIDMaxBytes.
//
// Everything surviving those rules is accepted verbatim: matching is
// byte-exact and case-sensitive, and no Unicode normalization is performed.
func NormalizeExternalID(raw string) (string, error) {
	for _, r := range raw {
		if isASCIIControl(r) {
			return "", fmt.Errorf("%w: control characters are not allowed", ErrExternalIDInvalid)
		}
	}
	trimmed := strings.TrimFunc(raw, unicode.IsSpace)
	if trimmed == "" {
		return "", nil
	}
	if len(trimmed) > ExternalIDMaxBytes {
		return "", fmt.Errorf("%w: must be %d UTF-8 bytes or fewer", ErrExternalIDInvalid, ExternalIDMaxBytes)
	}
	return trimmed, nil
}

func isASCIIControl(r rune) bool {
	return (r >= 0x00 && r <= 0x1F) || r == 0x7F
}

// normalizeRequiredExternalID is NormalizeExternalID plus a "missing" check.
// The REST lookup and release routes take external_id as their sole
// identifying parameter, unlike create (where an absent external_id just
// skips idempotency) — these routes have nothing to do without one, so an
// empty result after normalization is itself a validation failure.
func normalizeRequiredExternalID(raw string) (string, error) {
	normalized, err := NormalizeExternalID(raw)
	if err != nil {
		return "", err
	}
	if normalized == "" {
		return "", fmt.Errorf("%w: external_id is required", ErrExternalIDInvalid)
	}
	return normalized, nil
}

// GetTaskByExternalID is the REST lookup route's service-level
// implementation: a side-effect-free way to ask what holds an identity
// without risking a create. Returns the task holding (workspaceID,
// externalID) — including archived and unsettled tasks — or
// taskrepo.ErrTaskNotFound. Authorization precedes validation, matching the
// create path's ordering: an unauthorized caller learns nothing about
// whether the identity exists.
func (s *Service) GetTaskByExternalID(ctx context.Context, workspaceID, rawExternalID string) (*models.Task, error) {
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return nil, err
	}
	externalID, err := normalizeRequiredExternalID(rawExternalID)
	if err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTaskByExternalID(ctx, workspaceID, externalID)
	if err != nil {
		return nil, err
	}
	s.hydrateTaskRelations(ctx, task)
	return task, nil
}

// ReleaseTaskExternalID is the REST release route's service-level
// implementation: an operator action that frees an identity without
// deleting the task holding it (docs/specs/tasks/external-id-idempotency/spec.md,
// "The one unsafe thing a caller can do" — this is for a human who has
// determined the task is abandoned, not an automated retry loop). Returns
// whether a task held the identity. Publishes task.updated on success, like
// every other task mutation (apps/backend/AGENTS.md, "Task lifecycle
// events") — external_id is part of the task DTO and lifecycle-event
// contract, so a WS-connected client would otherwise keep a stale copy.
func (s *Service) ReleaseTaskExternalID(ctx context.Context, workspaceID, rawExternalID string) (bool, error) {
	if err := s.authorizeWorkspaceID(ctx, workspaceID); err != nil {
		return false, err
	}
	externalID, err := normalizeRequiredExternalID(rawExternalID)
	if err != nil {
		return false, err
	}
	task, err := s.tasks.ReleaseTaskExternalID(ctx, workspaceID, externalID)
	if err != nil {
		return false, err
	}
	if task == nil {
		return false, nil
	}
	s.PublishTaskUpdated(ctx, task)
	return true, nil
}

// SettleExternalID performs the create sequence's step 7 (settlement) for a
// task the caller just created (docs/specs/tasks/external-id-idempotency/spec.md,
// "Settlement"). Handlers call this after their surface's required
// synchronous work finishes and before any asynchronous dispatch. Safe to
// call unconditionally: a request with no external_id (externalID == "")
// trivially reports settled=true so callers don't need a branch for the
// common case.
//
// Returns:
//   - settled=true: exactly one row was updated (or there was nothing to
//     settle); the caller may proceed to asynchronous dispatch.
//   - settled=false, err=nil: zero rows were affected because the identity
//     was released while this create was running. survivor is the task,
//     which still exists but holds no external_id — the caller must report
//     outcome CreatedIdentityLost and MUST NOT dispatch asynchronous work.
//   - err wrapping taskrepo.ErrTaskNotFound: zero rows were affected because
//     the task was deleted while this create was running. The caller must
//     surface its surface's existing not-found error and MUST NOT dispatch
//     asynchronous work.
func (s *Service) SettleExternalID(ctx context.Context, taskID, externalID string) (settled bool, survivor *models.Task, err error) {
	if externalID == "" {
		return true, nil, nil
	}
	ok, err := s.tasks.SettleTaskExternalID(ctx, taskID, externalID, time.Now().UTC())
	if err != nil {
		return false, nil, err
	}
	if ok {
		return true, nil, nil
	}
	survivor, err = s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return false, nil, err
	}
	s.hydrateTaskRelations(ctx, survivor)
	return false, survivor, nil
}
