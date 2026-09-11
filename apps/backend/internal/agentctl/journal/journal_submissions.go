package journal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

func (j *Journal) PutSubmission(ctx context.Context, submission Submission) (Submission, error) {
	stored, _, err := j.PutSubmissionWithResult(ctx, submission)
	return stored, err
}

// PutSubmissionWithResult records an immutable submission and reports whether
// the ID already existed. Callers use the identity to distinguish first
// admission, which may dispatch, from a retry, which must only return the
// durable state.
func (j *Journal) PutSubmissionWithResult(ctx context.Context, submission Submission) (Submission, bool, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if submission.ID == "" || submission.Hash == "" {
		return Submission{}, false, fmt.Errorf("submission id and hash are required")
	}
	if int64(len(submission.Payload)) > j.config.MaxEventBytes {
		return Submission{}, false, fmt.Errorf("%w: %d bytes", ErrStreamFull, len(submission.Payload))
	}
	if submission.CreatedAt.IsZero() {
		submission.CreatedAt = time.Now().UTC()
	}
	if submission.UpdatedAt.IsZero() {
		submission.UpdatedAt = submission.CreatedAt
	}
	if submission.State == "" {
		submission.State = SubmissionPrepared
	}
	duplicate := false
	err := j.db.Update(func(tx *bolt.Tx) error {
		stored, isDuplicate, err := putSubmissionTx(ctx, tx, submission, j)
		if err != nil {
			return err
		}
		submission = stored
		duplicate = isDuplicate
		return nil
	})
	if duplicate {
		RecordDuplicateSubmission("same_hash")
	}
	if errors.Is(err, ErrSubmissionConflict) {
		RecordDuplicateSubmission("hash_conflict")
	}
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		RecordJournalError(classifyJournalError(err))
	}
	return submission, duplicate, err
}

func putSubmissionTx(ctx context.Context, tx *bolt.Tx, submission Submission, j *Journal) (Submission, bool, error) {
	if err := ctx.Err(); err != nil {
		return Submission{}, false, err
	}
	bucket := tx.Bucket(bucketSubmissions)
	if raw := bucket.Get([]byte(submission.ID)); raw != nil {
		var existing Submission
		if err := json.Unmarshal(raw, &existing); err != nil {
			return Submission{}, false, ErrJournalCorrupt
		}
		if existing.Hash != submission.Hash {
			return Submission{}, false, ErrSubmissionConflict
		}
		return existing, true, nil
	}
	encoded, err := json.Marshal(submission)
	if err != nil {
		return Submission{}, false, err
	}
	journalBytes, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
	if err != nil {
		return Submission{}, false, err
	}
	if err := validateJournalCapacity(j, journalBytes, int64(len(encoded))); err != nil {
		return Submission{}, false, err
	}
	if err := bucket.Put([]byte(submission.ID), encoded); err != nil {
		return Submission{}, false, err
	}
	if err := tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(journalBytes+int64(len(encoded)))); err != nil {
		return Submission{}, false, err
	}
	return submission, false, nil
}

func markSubmissionTerminalTx(ctx context.Context, tx *bolt.Tx, id string, sequence uint64, j *Journal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	bucket := tx.Bucket(bucketSubmissions)
	raw := bucket.Get([]byte(id))
	if raw == nil {
		return ErrSubmissionNotFound
	}
	var submission Submission
	if err := json.Unmarshal(raw, &submission); err != nil {
		return ErrJournalCorrupt
	}
	if submission.TerminalEventRetained {
		if submission.TerminalSequence != sequence {
			return ErrSequenceConflict
		}
		return nil
	}
	oldBytes := len(raw)
	submission.TerminalEventRetained = true
	submission.TerminalSequence = sequence
	submission.UpdatedAt = time.Now().UTC()
	encoded, err := json.Marshal(submission)
	if err != nil {
		return err
	}
	journalBytes, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
	if err != nil {
		return err
	}
	if err := validateJournalCapacity(j, journalBytes-int64(oldBytes), int64(len(encoded))-int64(oldBytes)); err != nil {
		return err
	}
	if err := bucket.Put([]byte(id), encoded); err != nil {
		return err
	}
	return tx.Bucket(bucketMeta).Put(
		keyJournalBytes,
		encodeInt64(journalBytes+int64(len(encoded))-int64(oldBytes)),
	)
}

func (j *Journal) GetSubmission(ctx context.Context, id string) (Submission, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var submission Submission
	err := j.db.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw := tx.Bucket(bucketSubmissions).Get([]byte(id))
		if raw == nil {
			return ErrSubmissionNotFound
		}
		return json.Unmarshal(raw, &submission)
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		RecordJournalError(classifyJournalError(err))
	}
	return submission, err
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

// HasUnresolvedWork reports whether the retained journal contains work that
// cannot be silently downgraded to a legacy delivery path. It covers both
// prompt outcomes and event records that still need backend acknowledgment.
func (j *Journal) HasUnresolvedWork(ctx context.Context) (bool, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var unresolved bool
	err := j.db.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := tx.Bucket(bucketSubmissions).ForEach(func(_, raw []byte) error {
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
		}); err != nil || unresolved {
			return err
		}
		return tx.Bucket(bucketStreamMeta).ForEach(func(_, raw []byte) error {
			if raw == nil {
				return nil
			}
			stream, err := decodeStream(raw)
			if err != nil {
				return err
			}
			if stream.HighWater > stream.Acknowledged {
				unresolved = true
			}
			return nil
		})
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		RecordJournalError(classifyJournalError(err))
	}
	return unresolved, err
}

func (j *Journal) TransitionSubmission(ctx context.Context, id string, next SubmissionState, updatedAt time.Time) (Submission, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var submission Submission
	err := j.db.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		bucket := tx.Bucket(bucketSubmissions)
		raw := bucket.Get([]byte(id))
		if raw == nil {
			return ErrSubmissionNotFound
		}
		if err := json.Unmarshal(raw, &submission); err != nil {
			return ErrJournalCorrupt
		}
		if !validSubmissionTransition(submission.State, next) {
			return ErrSubmissionState
		}
		oldBytes := len(raw)
		submission.State = next
		if updatedAt.IsZero() {
			updatedAt = time.Now().UTC()
		}
		submission.UpdatedAt = updatedAt
		encoded, err := json.Marshal(submission)
		if err != nil {
			return err
		}
		journalBytes, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
		if err != nil {
			return err
		}
		if err := validateJournalCapacity(j, journalBytes-int64(oldBytes), int64(len(encoded))-int64(oldBytes)); err != nil {
			return err
		}
		if err := bucket.Put([]byte(id), encoded); err != nil {
			return err
		}
		return tx.Bucket(bucketMeta).Put(
			keyJournalBytes,
			encodeInt64(journalBytes+int64(len(encoded))-int64(oldBytes)),
		)
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		RecordJournalError(classifyJournalError(err))
	}
	return submission, err
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
		journalBytes, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
		if err != nil {
			return err
		}
		if journalBytes < int64(len(raw)) {
			return ErrJournalCorrupt
		}
		if err := bucket.Delete([]byte(id)); err != nil {
			return err
		}
		return tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(journalBytes-int64(len(raw))))
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		RecordJournalError(classifyJournalError(err))
	}
	if err == nil {
		j.refreshMetrics()
	}
	return retired, err
}
