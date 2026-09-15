package process

import (
	"context"
	"errors"
	"fmt"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

var ErrSubmissionUncertain = errors.New("agent delivery submission outcome is uncertain")

type deterministicPromptFailure interface {
	DeterministicPromptFailure() bool
}

func isKnownSubmissionFailure(err error) bool {
	var requestErr *acp.RequestError
	if errors.As(err, &requestErr) {
		return true
	}
	var providerFailure deterministicPromptFailure
	return errors.As(err, &providerFailure) && providerFailure.DeterministicPromptFailure()
}

// SubmissionDelivery coordinates the agentctl-side immutable submission
// record. It is deliberately independent of the ACP prompt call so a caller
// can reconcile durable state before deciding whether a harness call is safe.
type SubmissionDelivery struct {
	Journal *journal.Journal
	Now     func() time.Time
}

func (d *SubmissionDelivery) Admit(ctx context.Context, submission journal.Submission) (journal.Submission, error) {
	if d == nil || d.Journal == nil {
		return journal.Submission{}, fmt.Errorf("submission journal is required")
	}
	if submission.State == "" {
		submission.State = journal.SubmissionPrepared
	}
	stored, err := d.Journal.PutSubmission(ctx, submission)
	if err != nil {
		return journal.Submission{}, err
	}
	if stored.State == journal.SubmissionPrepared {
		stored, err = d.Journal.TransitionSubmission(ctx, stored.ID, journal.SubmissionAccepted, d.now())
		if err != nil {
			return journal.Submission{}, err
		}
	}
	journal.RecordSubmission(string(stored.State))
	return stored, nil
}

// Dispatch moves accepted to dispatching before invoking the harness. An
// existing dispatching record is never retried automatically because the
// external call may already have happened.
func (d *SubmissionDelivery) Dispatch(ctx context.Context, id string, call func(context.Context) error) (journal.Submission, error) {
	if d == nil || d.Journal == nil || call == nil {
		return journal.Submission{}, fmt.Errorf("submission journal and dispatch callback are required")
	}
	submission, shouldDispatch, err := d.prepareDispatch(ctx, id)
	if err != nil {
		return journal.Submission{}, err
	}
	if !shouldDispatch {
		return submission, nil
	}
	if err := call(ctx); err != nil {
		return d.handleDispatchError(ctx, id, err)
	}
	completed, err := d.Journal.TransitionSubmission(ctx, id, journal.SubmissionCompleted, d.now())
	if err != nil {
		return journal.Submission{}, err
	}
	return completed, nil
}

func (d *SubmissionDelivery) prepareDispatch(ctx context.Context, id string) (journal.Submission, bool, error) {
	submission, err := d.Journal.GetSubmission(ctx, id)
	if err != nil {
		return journal.Submission{}, false, err
	}
	if submission.Retired {
		journal.RecordUncertainSubmission("retired_submission")
		return submission, false, ErrSubmissionUncertain
	}
	switch submission.State {
	case journal.SubmissionCompleted, journal.SubmissionFailed, journal.SubmissionCancelled:
		return submission, false, nil
	case journal.SubmissionInterruptedUnknown, journal.SubmissionDispatching:
		journal.RecordUncertainSubmission("preexisting_uncertain")
		return submission, false, ErrSubmissionUncertain
	case journal.SubmissionAccepted:
		_, err := d.Journal.TransitionSubmission(ctx, id, journal.SubmissionDispatching, d.now())
		return submission, true, err
	default:
		return submission, false, fmt.Errorf("submission %s is not accepted: %s", id, submission.State)
	}
}

func (d *SubmissionDelivery) handleDispatchError(ctx context.Context, id string, dispatchErr error) (journal.Submission, error) {
	if isKnownSubmissionFailure(dispatchErr) {
		failed, transitionErr := d.Journal.TransitionSubmission(ctx, id, journal.SubmissionFailed, d.now())
		if transitionErr != nil {
			return journal.Submission{}, errors.Join(ErrSubmissionUncertain, transitionErr)
		}
		return failed, dispatchErr
	}
	unknown, transitionErr := d.Journal.TransitionSubmission(ctx, id, journal.SubmissionInterruptedUnknown, d.now())
	journal.RecordUncertainSubmission("dispatch_error")
	if transitionErr != nil {
		return journal.Submission{}, errors.Join(ErrSubmissionUncertain, transitionErr)
	}
	return unknown, fmt.Errorf("dispatch outcome is unknown: %w", dispatchErr)
}

func (d *SubmissionDelivery) now() time.Time {
	if d.Now != nil {
		return d.Now().UTC()
	}
	return time.Now().UTC()
}
