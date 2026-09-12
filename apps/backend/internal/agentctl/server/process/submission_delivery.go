package process

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

var ErrSubmissionUncertain = errors.New("agent delivery submission outcome is uncertain")

// SubmissionDelivery coordinates the agentctl-side immutable submission
// record. It is deliberately independent of the ACP prompt call so a caller
// can reconcile durable state before deciding whether a harness call is safe.
type SubmissionDelivery struct {
	Journal *journal.Journal
	Now     func() time.Time
}

func (d *SubmissionDelivery) Admit(ctx context.Context, submission journal.Submission) (journal.Submission, error) {
	stored, _, err := d.AdmitWithResult(ctx, submission)
	return stored, err
}

// AdmitWithResult returns whether the immutable submission was already
// present. A duplicate is a reconciliation response, not a new dispatch
// opportunity, even when its state is only accepted.
func (d *SubmissionDelivery) AdmitWithResult(ctx context.Context, submission journal.Submission) (journal.Submission, bool, error) {
	if d == nil || d.Journal == nil {
		return journal.Submission{}, false, fmt.Errorf("submission journal is required")
	}
	if submission.State == "" {
		submission.State = journal.SubmissionPrepared
	}
	stored, duplicate, err := d.Journal.PutSubmissionWithResult(ctx, submission)
	if err != nil {
		return journal.Submission{}, false, err
	}
	if !duplicate && stored.State == journal.SubmissionPrepared {
		stored, err = d.Journal.TransitionSubmission(ctx, stored.ID, journal.SubmissionAccepted, d.now())
		if err != nil {
			return journal.Submission{}, false, err
		}
	}
	journal.RecordSubmission(string(stored.State))
	return stored, duplicate, nil
}

// Dispatch moves accepted to dispatching before invoking the harness. An
// existing dispatching record is never retried automatically because the
// external call may already have happened.
func (d *SubmissionDelivery) Dispatch(ctx context.Context, id string, call func(context.Context) error) (journal.Submission, error) {
	if d == nil || d.Journal == nil || call == nil {
		return journal.Submission{}, fmt.Errorf("submission journal and dispatch callback are required")
	}
	submission, err := d.Journal.GetSubmission(ctx, id)
	if err != nil {
		return journal.Submission{}, err
	}
	switch submission.State {
	case journal.SubmissionCompleted, journal.SubmissionFailed, journal.SubmissionCancelled:
		return submission, nil
	case journal.SubmissionInterruptedUnknown, journal.SubmissionDispatching:
		journal.RecordUncertainSubmission("preexisting_uncertain")
		return submission, ErrSubmissionUncertain
	case journal.SubmissionAccepted:
		_, transitionErr := d.Journal.TransitionSubmission(ctx, id, journal.SubmissionDispatching, d.now())
		if transitionErr != nil {
			return journal.Submission{}, transitionErr
		}
	default:
		return submission, fmt.Errorf("submission %s is not accepted: %s", id, submission.State)
	}
	if err := call(ctx); err != nil {
		unknown, transitionErr := d.Journal.TransitionSubmission(ctx, id, journal.SubmissionInterruptedUnknown, d.now())
		journal.RecordUncertainSubmission("dispatch_error")
		if transitionErr != nil {
			return journal.Submission{}, errors.Join(ErrSubmissionUncertain, transitionErr)
		}
		return unknown, fmt.Errorf("dispatch outcome is unknown: %w", err)
	}
	completed, err := d.Journal.TransitionSubmission(ctx, id, journal.SubmissionCompleted, d.now())
	if err != nil {
		return journal.Submission{}, err
	}
	return completed, nil
}

func (d *SubmissionDelivery) now() time.Time {
	if d.Now != nil {
		return d.Now().UTC()
	}
	return time.Now().UTC()
}
