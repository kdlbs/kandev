package journal

import (
	"context"
	"encoding/json"
	"errors"

	bolt "go.etcd.io/bbolt"
)

// HasUnresolvedSubmissions reports whether a prompt submission has an outcome
// that cannot be safely assumed by a new prompt admission. Stream events are
// intentionally excluded because they are replayable rather than uncertain.
func (j *Journal) HasUnresolvedSubmissions(ctx context.Context) (bool, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var unresolved bool
	err := j.db.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return tx.Bucket(bucketSubmissions).ForEach(func(_, raw []byte) error {
			if raw == nil {
				return nil
			}
			var submission Submission
			if err := json.Unmarshal(raw, &submission); err != nil {
				return ErrJournalCorrupt
			}
			switch submission.State {
			case SubmissionPrepared, SubmissionAccepted, SubmissionDispatching, SubmissionInterruptedUnknown:
				unresolved = true
			case SubmissionCompleted:
				if !submission.TerminalEventRetained {
					unresolved = true
				}
			}
			return nil
		})
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		RecordJournalError(classifyJournalError(err))
	}
	return unresolved, err
}

// ListSubmissions returns submissions owned by one Kandev session. The
// session filter is used by an explicitly authorized generation transition to
// identify older unresolved prompts without exposing another session's data.
func (j *Journal) ListSubmissions(ctx context.Context, sessionID string) ([]Submission, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var submissions []Submission
	err := j.db.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return tx.Bucket(bucketSubmissions).ForEach(func(_, raw []byte) error {
			if raw == nil {
				return nil
			}
			var submission Submission
			if err := json.Unmarshal(raw, &submission); err != nil {
				return ErrJournalCorrupt
			}
			if sessionID == "" || submission.SessionID == sessionID {
				submissions = append(submissions, submission)
			}
			return nil
		})
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		RecordJournalError(classifyJournalError(err))
	}
	return submissions, err
}

// RetireSubmission removes one explicitly recovered submission. A newer
// harness generation must authorize the removal, so an uncertain dispatch
// cannot be silently retried by the same generation.
func (j *Journal) RetireSubmission(ctx context.Context, id string, recoveryGeneration uint64) (Submission, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var retired Submission
	err := j.db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		bucket := tx.Bucket(bucketSubmissions)
		raw := bucket.Get([]byte(id))
		if raw == nil {
			return ErrSubmissionNotFound
		}
		if err := json.Unmarshal(raw, &retired); err != nil {
			return ErrJournalCorrupt
		}
		if recoveryGeneration == 0 || recoveryGeneration <= retired.HarnessGeneration {
			return ErrSubmissionGeneration
		}
		return bucket.Delete([]byte(id))
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		RecordJournalError(classifyJournalError(err))
	}
	if err == nil {
		j.refreshMetrics()
	}
	return retired, err
}
