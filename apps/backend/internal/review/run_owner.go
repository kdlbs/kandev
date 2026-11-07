package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type runProfileLookup interface {
	NativeRouteOwnersEnabled() bool
	ReviewerProfileForRun(context.Context, string, string) (ReviewerProfile, bool, error)
}

type ownedProfileLookup struct {
	lookup runProfileLookup
	runID  string
}

func (l ownedProfileLookup) ReviewerProfile(ctx context.Context, profileID string) (ReviewerProfile, bool, error) {
	return l.lookup.ReviewerProfileForRun(ctx, profileID, l.runID)
}

func (r *Resolver) resolveForRun(ctx context.Context, runID string, identity ReviewerIdentity) (ReviewerIdentity, bool, error) {
	lookup, ok := r.Profiles.(runProfileLookup)
	if !ok || !lookup.NativeRouteOwnersEnabled() || identity.ProfileID == "" {
		return identity, false, nil
	}
	owned := *r
	owned.Profiles = ownedProfileLookup{lookup: lookup, runID: runID}
	resolved, err := owned.resolveProfile(ctx, identity.ProfileID, identity.Source == SourceAgentProfile)
	resolved.Source = identity.Source
	return resolved, true, err
}

type runReviewerAttributor interface {
	SetRunReviewer(context.Context, string, string, string) (*models.TaskReviewRun, error)
}

// bindRunReviewer is the run-scoped selection. The initial lookup is read-only;
// the durable run row exists before this method persists the concrete profile.
func (r *Runner) bindRunReviewer(ctx context.Context, run *models.TaskReviewRun, identity ReviewerIdentity) (*models.TaskReviewRun, ReviewerIdentity, error) {
	runID := run.ID
	resolved, bound, err := r.resolver.resolveForRun(ctx, runID, identity)
	if err == nil && !bound {
		return run, identity, nil
	}
	if err == nil {
		writer, ok := r.store.(runReviewerAttributor)
		if !ok {
			err = fmt.Errorf("review run attribution is unavailable")
		} else {
			run, err = writer.SetRunReviewer(ctx, run.ID, resolved.AgentID, resolved.Model)
			if err == nil {
				return run, resolved, nil
			}
		}
	}
	var wait nativeAdmissionWaiter
	if errors.As(err, &wait) {
		return run, identity, err
	}
	return nil, ReviewerIdentity{}, r.failRunPreparation(ctx, runID, err)
}

func (r *Runner) failRunPreparation(ctx context.Context, runID string, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if errors.Is(cause, context.Canceled) {
		_, err := r.store.CancelRun(cleanupCtx, runID)
		return errors.Join(cause, err)
	}
	return r.fail(cleanupCtx, runID, cause, time.Now())
}

type ownedRunStarter interface {
	StartOwnedRun(context.Context, string) (bool, error)
}

func (r *Runner) startInferenceRun(ctx context.Context, runID string, identity ReviewerIdentity) (bool, error) {
	if identity.Route != nil && strings.HasPrefix(identity.Route.SessionID, "native:review-run:") {
		starter, ok := r.store.(ownedRunStarter)
		if !ok {
			return false, fmt.Errorf("native review run start claim is unavailable")
		}
		return starter.StartOwnedRun(ctx, runID)
	}
	_, err := r.store.MarkRunRunning(ctx, runID)
	return err == nil, err
}

type reviewSelectionCompleter interface {
	CompleteSelection(context.Context, ReviewerIdentity)
}

func (r *Runner) completeVerifiedSelection(ctx context.Context, completed *models.TaskReviewRun, identity ReviewerIdentity) {
	if completed == nil || completed.Status != models.ReviewRunCompleted {
		return
	}
	if complete, ok := r.inference.(reviewSelectionCompleter); ok {
		complete.CompleteSelection(ctx, identity)
	}
}
