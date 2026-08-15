package github

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

// ErrInvalidDisposition signals that a caller-supplied disposition body was
// rejected: an unknown enum value, a superseded_by_url paired with a
// non-superseded disposition, or a superseded_by_url that resolves to the
// association's own PR.
var ErrInvalidDisposition = errors.New("invalid task PR disposition")

// SetTaskPRDisposition records, updates, or clears a human-supplied closure
// reason for a task-PR association. Order of operations mirrors
// DetachTaskPR: the association is looked up and authorized before any
// validation, so a superseded_by_url self-reference check has the
// association's own identity available and authorization matches every
// other task-PR mutation.
//
// patch is merged onto the stored row (mergeTaskPRDispositionPatch) before
// validation: a field absent from the patch preserves its stored value, so
// an empty patch is a true no-op (not a clear) and a URL-only patch can
// update an already-superseded disposition's URL without resending
// "disposition". Validation then runs against the RESULTING merged state
// (validateTaskPRDisposition), not just the fields this patch happened to
// touch, so changing disposition away from "superseded" while an old
// superseded_by_url is still on the row is rejected unless the same patch
// also clears the URL. Explicit `null` on disposition always clears all
// three disposition columns (AC-22), overriding whatever the stored
// superseded_by_url was. The endpoint accepts the write regardless of the
// association's state (AC-29b) or detached_at (AC-27): a detached
// association is exactly a PR someone walked away from, and a user may
// record intent on a PR they are about to close.
func (s *Service) SetTaskPRDisposition(
	ctx context.Context, workspaceID, associationID string, patch DispositionPatch,
) (*TaskPR, error) {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(associationID) == "" {
		return nil, ErrTaskPRNotFound
	}
	tp, err := s.store.GetTaskPRByID(ctx, associationID)
	if err != nil {
		return nil, err
	}
	if tp == nil {
		return nil, ErrTaskPRNotFound
	}
	resolvedWorkspaceID, err := s.authorizeTaskPRMutation(ctx, tp, workspaceID)
	if err != nil {
		return nil, err
	}
	if !patch.HasAny() {
		return tp, nil
	}
	patch, err = normalizeDispositionPatch(patch)
	if err != nil {
		return nil, err
	}

	disposition, supersededByURL := mergeTaskPRDispositionPatch(tp, patch)
	if err := validateTaskPRDisposition(tp, disposition, supersededByURL); err != nil {
		return nil, err
	}

	if dispositionUnchanged(tp, disposition, supersededByURL) {
		return tp, nil
	}
	action := "clear"
	if disposition != nil {
		action = "set"
	}
	return s.writeTaskPRDisposition(ctx, associationID, patch, action, resolvedWorkspaceID)
}

// normalizeDispositionPatch puts the raw caller-supplied DispositionPatch
// into the exact shape that gets both validated and persisted (the patch
// itself is now the write, per Store.UpdateTaskPRDisposition — SEC-001 — so
// unlike the old merge-then-write flow there is no second normalization
// pass between validation and the store call):
//
//  1. An explicit superseded_by_url is trimmed, with an empty string
//     normalized to absent (nil) — the same rule this endpoint has always
//     applied to the merged value, now applied to the patch that is
//     actually written.
//  2. A request that explicitly sets BOTH `disposition: null` and a non-nil
//     `superseded_by_url` in the same body is rejected (AC-24): "nobody
//     looked" and "here is the superseding PR" cannot both be true, and
//     silently discarding one of the two caller-supplied fields would mask
//     a client bug instead of surfacing it.
//  3. Otherwise, an explicit `disposition: null` still forces
//     superseded_by_url to clear too (AC-22), regardless of whether the
//     same request also touched superseded_by_url. Without this, a
//     disposition-only null PATCH sent against a row with a previously
//     stored superseded_by_url would merge to (disposition: nil,
//     superseded_by_url: non-nil) — a combination validateTaskPRDisposition
//     rejects — instead of restoring the "nobody looked" state the caller
//     asked for.
func normalizeDispositionPatch(patch DispositionPatch) (DispositionPatch, error) {
	if patch.SupersededByURLSet {
		patch.SupersededByURL = normalizeSupersededByURL(patch.SupersededByURL)
	}
	if patch.DispositionSet && patch.Disposition == nil {
		if patch.SupersededByURLSet && patch.SupersededByURL != nil {
			return DispositionPatch{}, ErrInvalidDisposition
		}
		patch.SupersededByURLSet = true
		patch.SupersededByURL = nil
	}
	return patch, nil
}

// writeTaskPRDisposition persists a validated, changed disposition write,
// re-reads the row, and publishes the update event. Split out of
// SetTaskPRDisposition to keep that function's complexity within the repo's
// lint limits. The write itself is the normalized patch, not the merged
// final values SetTaskPRDisposition computed for validation — see
// Store.UpdateTaskPRDisposition for why (SEC-001).
func (s *Service) writeTaskPRDisposition(
	ctx context.Context, associationID string, patch DispositionPatch, action, workspaceID string,
) (*TaskPR, error) {
	if err := s.store.UpdateTaskPRDisposition(ctx, associationID, patch); err != nil {
		return nil, err
	}
	incTaskPROutcomeDisposition(action)

	updated, err := s.store.GetTaskPRByID(ctx, associationID)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, ErrTaskPRNotFound
	}
	if s.eventBus != nil {
		eventPayload := *updated
		eventPayload.WorkspaceID = workspaceID
		event := bus.NewEvent(events.GitHubTaskPRUpdated, "github", &eventPayload)
		if err := s.eventBus.Publish(ctx, events.GitHubTaskPRUpdated, event); err != nil {
			s.logger.Debug("failed to publish task PR updated event", zap.Error(err))
		}
	}
	return updated, nil
}

// mergeTaskPRDispositionPatch merges a partial disposition patch onto the
// stored association: a field the patch did not set (DispositionSet /
// SupersededByURLSet false) preserves tp's stored value; a field the patch
// did set uses the patch's value as-is (nil for an explicit `null`). An
// explicit superseded_by_url is trimmed, with an empty string normalized to
// absent (nil) — the same "empty string is absent, not an invalid URL" rule
// this endpoint has always applied.
func mergeTaskPRDispositionPatch(tp *TaskPR, patch DispositionPatch) (disposition, supersededByURL *string) {
	disposition = tp.Disposition
	if patch.DispositionSet {
		disposition = patch.Disposition
	}
	supersededByURL = tp.DispositionSupersededByURL
	if patch.SupersededByURLSet {
		supersededByURL = normalizeSupersededByURL(patch.SupersededByURL)
	}
	return disposition, supersededByURL
}

// normalizeSupersededByURL trims raw and normalizes an empty result to nil,
// so an empty string is treated as absent rather than an invalid URL.
func normalizeSupersededByURL(raw *string) *string {
	if raw == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// validateTaskPRDisposition validates the RESULTING (already-merged)
// disposition and supersededByURL against tp's identity — not just the
// fields a particular patch happened to touch — so a patch that changes
// disposition away from "superseded" while an old superseded_by_url is still
// on the row is rejected exactly as if that URL had been freshly supplied.
func validateTaskPRDisposition(tp *TaskPR, disposition, supersededByURL *string) error {
	if disposition != nil && !validTaskPRDisposition(*disposition) {
		return ErrInvalidDisposition
	}
	if supersededByURL == nil {
		return nil
	}
	if disposition == nil || *disposition != TaskPRDispositionSuperseded {
		return ErrInvalidDisposition
	}
	owner, repo, number, err := parsePRURL(*supersededByURL)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidPRURL, err)
	}
	if strings.EqualFold(owner, tp.Owner) && strings.EqualFold(repo, tp.Repo) && number == tp.PRNumber {
		return ErrInvalidDisposition
	}
	return nil
}

// dispositionUnchanged reports whether the desired write is identical to
// what is already stored, so SetTaskPRDisposition can write nothing,
// publish nothing, and leave disposition_recorded_at byte-identical on a
// repeated PATCH (AC-29).
func dispositionUnchanged(tp *TaskPR, disposition, supersededByURL *string) bool {
	return stringPtrEqual(tp.Disposition, disposition) && stringPtrEqual(tp.DispositionSupersededByURL, supersededByURL)
}
