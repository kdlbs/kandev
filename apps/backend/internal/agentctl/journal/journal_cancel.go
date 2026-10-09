package journal

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

// CancelSubmission commits explicit cancellation and its replayable terminal together.
// A repeated request preserves the original terminal sequence.
func (j *Journal) CancelSubmission(ctx context.Context, id string, event Event) error {
	j.mu.RLock()
	defer j.mu.RUnlock()
	events := []Event{event}
	if err := prepareAppendEvents(events, j.config.MaxEventBytes); err != nil {
		return err
	}
	event = events[0]
	err := j.db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw := tx.Bucket(bucketSubmissions).Get([]byte(id))
		if raw == nil {
			return ErrSubmissionNotFound
		}
		var submission Submission
		if err := json.Unmarshal(raw, &submission); err != nil {
			return ErrJournalCorrupt
		}
		if submission.TerminalEventRetained || submission.State == SubmissionCompleted || submission.State == SubmissionFailed {
			return nil
		}
		if !cancelEventMatchesSubmission(event, submission) {
			return ErrOwnerMismatch
		}
		if _, err := j.transitionSubmissionTx(ctx, tx, id, SubmissionCancelled, time.Now().UTC()); err != nil {
			return err
		}
		return j.appendEventTx(ctx, tx, &event)
	})
	if err == nil {
		j.refreshMetricsLocked()
	} else if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		RecordJournalError(classifyJournalError(err))
	}
	return err
}

func cancelEventMatchesSubmission(event Event, submission Submission) bool {
	return event.SubmissionID == submission.ID && event.SessionID == submission.SessionID &&
		event.IncarnationID == submission.IncarnationID && event.HarnessGeneration == submission.HarnessGeneration &&
		event.StreamID == submission.StreamID && event.Terminal
}
