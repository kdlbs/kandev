package journal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
		retired.Retired = true
		retired.State = SubmissionCancelled
		retired.Payload = nil
		retired.UpdatedAt = time.Now().UTC()
		encoded, err := json.Marshal(retired)
		if err != nil {
			return err
		}
		journalBytes, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
		if err != nil {
			return err
		}
		journalBytes -= int64(len(raw)) - int64(len(encoded))
		if journalBytes < 0 {
			journalBytes = 0
		}
		if err := bucket.Put([]byte(id), encoded); err != nil {
			return err
		}
		return tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(journalBytes))
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		RecordJournalError(classifyJournalError(err))
	}
	if err == nil {
		j.refreshMetrics()
	}
	return retired, err
}

func (j *Journal) PutSubmission(ctx context.Context, submission Submission) (Submission, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if submission.ID == "" || submission.Hash == "" {
		return Submission{}, fmt.Errorf("submission id and hash are required")
	}
	if int64(len(submission.Payload)) > j.config.MaxEventBytes {
		return Submission{}, fmt.Errorf("%w: %d bytes", ErrStreamFull, len(submission.Payload))
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
		journalBytes, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
		if err != nil {
			return err
		}
		stored, isDuplicate, err := putSubmissionTx(ctx, tx, submission, journalBytes, j.config.MaxJournalBytes, j.config.ReserveBytes)
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
	return submission, err
}

func putSubmissionTx(ctx context.Context, tx *bolt.Tx, submission Submission, journalBytes, maxJournalBytes, reserveBytes int64) (Submission, bool, error) {
	if err := ctx.Err(); err != nil {
		return Submission{}, false, err
	}
	bucket := tx.Bucket(bucketSubmissions)
	existing, found, err := loadExistingSubmission(bucket, submission)
	if err != nil {
		return Submission{}, false, err
	}
	if found {
		return existing, true, nil
	}
	if submission.StreamID != "" {
		count, err := countStreamSubmissions(bucket, submission.StreamID)
		if err != nil {
			return Submission{}, false, err
		}
		if count >= MaxStreamSubmissions {
			return Submission{}, false, ErrStreamFull
		}
	}
	encoded, err := json.Marshal(submission)
	if err != nil {
		return Submission{}, false, err
	}
	if journalBytes+int64(len(encoded)) > maxJournalBytes-reserveBytes {
		return Submission{}, false, ErrJournalFull
	}
	if err := bucket.Put([]byte(submission.ID), encoded); err != nil {
		return Submission{}, false, err
	}
	if err := tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(journalBytes+int64(len(encoded)))); err != nil {
		return Submission{}, false, err
	}
	return submission, false, nil
}

func loadExistingSubmission(bucket *bolt.Bucket, submission Submission) (Submission, bool, error) {
	raw := bucket.Get([]byte(submission.ID))
	if raw == nil {
		return Submission{}, false, nil
	}
	var existing Submission
	if err := json.Unmarshal(raw, &existing); err != nil {
		return Submission{}, false, ErrJournalCorrupt
	}
	if existing.Hash != submission.Hash {
		return Submission{}, false, ErrSubmissionConflict
	}
	if existing.Retired {
		return Submission{}, false, ErrSubmissionState
	}
	return existing, true, nil
}

func countStreamSubmissions(bucket *bolt.Bucket, streamID string) (int, error) {
	var count int
	err := bucket.ForEach(func(_, raw []byte) error {
		if raw == nil {
			return nil
		}
		var stored Submission
		if err := json.Unmarshal(raw, &stored); err != nil {
			return ErrJournalCorrupt
		}
		if stored.StreamID == streamID {
			count++
		}
		return nil
	})
	return count, err
}

func markSubmissionTerminalTx(ctx context.Context, tx *bolt.Tx, id string, sequence uint64, maxJournalBytes int64) error {
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
	submission.TerminalEventRetained = true
	submission.TerminalSequence = sequence
	submission.UpdatedAt = time.Now().UTC()
	encoded, err := json.Marshal(submission)
	if err != nil {
		return err
	}
	oldBytes := int64(len(raw))
	newBytes := int64(len(encoded))
	journalBytes, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
	if err != nil {
		return err
	}
	if newBytes > oldBytes && journalBytes+newBytes-oldBytes > maxJournalBytes {
		return ErrJournalFull
	}
	if err := bucket.Put([]byte(id), encoded); err != nil {
		return err
	}
	return tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(journalBytes+newBytes-oldBytes))
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
		oldBytes := int64(len(raw))
		newBytes := int64(len(encoded))
		if newBytes > oldBytes && journalBytes+newBytes-oldBytes > j.config.MaxJournalBytes {
			return ErrJournalFull
		}
		if err := bucket.Put([]byte(id), encoded); err != nil {
			return err
		}
		return tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(journalBytes+newBytes-oldBytes))
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		RecordJournalError(classifyJournalError(err))
	}
	return submission, err
}
