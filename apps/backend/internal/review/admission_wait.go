package review

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
)

type nativeAdmissionWaiter interface{ AwaitNativeRoute(context.Context) error }

// prepareRunReviewer performs the run-scoped selection after the durable run row
// exists. A capacity wait is returned to the detached worker instead of being
// recorded as a failed run.
func (r *Runner) prepareRunReviewer(ctx context.Context, run *models.TaskReviewRun, identity ReviewerIdentity) (*models.TaskReviewRun, ReviewerIdentity, nativeAdmissionWaiter, error) {
	bound, resolved, err := r.bindRunReviewer(ctx, run, identity)
	var wait nativeAdmissionWaiter
	if errors.As(err, &wait) {
		return run, identity, wait, nil
	}
	if err != nil {
		r.resolver.releaseRunRoute(run.ID)
	}
	return bound, resolved, nil, err
}

func (r *Runner) executePreparedRun(ctx context.Context, req RunRequest, run *models.TaskReviewRun, identity ReviewerIdentity, wait nativeAdmissionWaiter, files []ChangedFile) error {
	defer r.resolver.releaseRunRoute(run.ID)
	for wait != nil {
		if err := wait.AwaitNativeRoute(ctx); err != nil {
			return r.failRunPreparation(ctx, run.ID, err)
		}
		var err error
		run, identity, wait, err = r.prepareRunReviewer(ctx, run, identity)
		if err != nil {
			return err
		}
	}
	return r.execute(ctx, req, run.ID, identity, files)
}

func (r *Resolver) releaseRunRoute(runID string) {
	if release, ok := r.Profiles.(interface{ ReleaseReviewRun(string) }); ok {
		release.ReleaseReviewRun(runID)
	}
}
