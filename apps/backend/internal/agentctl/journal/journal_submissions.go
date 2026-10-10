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
	err := j.viewLocked(func(tx *bolt.Tx) error {
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
			unresolved = unresolved || submissionNeedsRecovery(submission)
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
	err := j.viewLocked(func(tx *bolt.Tx) error {
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

// RetireSubmission seals an explicitly recovered submission. Native resume
// may acknowledge an interrupted unknown outcome in the same generation;
// retiring other work requires a newer generation.
func (j *Journal) RetireSubmission(ctx context.Context, id string, recoveryGeneration uint64) (Submission, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var retired Submission
	err := j.updateLocked(func(tx *bolt.Tx) error {
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
		if recoveryGeneration == 0 || recoveryGeneration < retired.HarnessGeneration ||
			(recoveryGeneration == retired.HarnessGeneration && retired.State != SubmissionInterruptedUnknown) {
			return ErrSubmissionGeneration
		}
		retired.Retired = true
		if retired.State != SubmissionInterruptedUnknown {
			retired.State = SubmissionCancelled
			retired.Payload = nil
		}
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
	err := j.updateLocked(func(tx *bolt.Tx) error {
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

func markSubmissionTerminalTx(ctx context.Context, tx *bolt.Tx, event Event, maxJournalBytes int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	bucket := tx.Bucket(bucketSubmissions)
	raw := bucket.Get([]byte(event.SubmissionID))
	if raw == nil {
		return ErrSubmissionNotFound
	}
	var submission Submission
	if err := json.Unmarshal(raw, &submission); err != nil {
		return ErrJournalCorrupt
	}
	if !submissionOwnsTerminal(submission, event) {
		return ErrOwnerMismatch
	}
	if submission.TerminalEventRetained {
		if submission.TerminalSequence != event.Sequence {
			return ErrSequenceConflict
		}
		return nil
	}
	// A definitive completion must release admission in the same commit that
	// makes its terminal event replayable. Preserve other recorded outcomes.
	if event.Type == "complete" && submission.State == SubmissionDispatching && !submission.Retired {
		submission.State = SubmissionCompleted
	}
	submission.TerminalEventRetained = true
	submission.TerminalSequence = event.Sequence
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
	if err := bucket.Put([]byte(event.SubmissionID), encoded); err != nil {
		return err
	}
	return tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(journalBytes+newBytes-oldBytes))
}

func submissionOwnsTerminal(submission Submission, event Event) bool {
	return submission.SessionID == event.SessionID && submission.IncarnationID == event.IncarnationID &&
		submission.HarnessGeneration == event.HarnessGeneration &&
		(submission.StreamID == "" || submission.StreamID == event.StreamID)
}

func (j *Journal) GetSubmission(ctx context.Context, id string) (Submission, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var submission Submission
	err := j.viewLocked(func(tx *bolt.Tx) error {
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
	err := j.viewLocked(func(tx *bolt.Tx) error {
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
			unresolved = unresolved || submissionNeedsRecovery(submission)
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
	err := j.updateLocked(func(tx *bolt.Tx) error {
		var err error
		submission, err = j.transitionSubmissionTx(ctx, tx, id, next, updatedAt)
		return err
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		RecordJournalError(classifyJournalError(err))
	}
	return submission, err
}

func (j *Journal) transitionSubmissionTx(ctx context.Context, tx *bolt.Tx, id string, next SubmissionState, updatedAt time.Time) (Submission, error) {
	var submission Submission
	if err := ctx.Err(); err != nil {
		return submission, err
	}
	bucket := tx.Bucket(bucketSubmissions)
	raw := bucket.Get([]byte(id))
	if raw == nil {
		return submission, ErrSubmissionNotFound
	}
	if err := json.Unmarshal(raw, &submission); err != nil {
		return submission, ErrJournalCorrupt
	}
	if !validSubmissionTransition(submission.State, next) {
		return submission, ErrSubmissionState
	}
	submission.State = next
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	submission.UpdatedAt = updatedAt
	encoded, err := json.Marshal(submission)
	if err != nil {
		return submission, err
	}
	journalBytes, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
	if err != nil {
		return submission, err
	}
	oldBytes := int64(len(raw))
	newBytes := int64(len(encoded))
	if newBytes > oldBytes && journalBytes+newBytes-oldBytes > j.config.MaxJournalBytes {
		return submission, ErrJournalFull
	}
	if err := bucket.Put([]byte(id), encoded); err != nil {
		return submission, err
	}
	return submission, tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(journalBytes+newBytes-oldBytes))
}
