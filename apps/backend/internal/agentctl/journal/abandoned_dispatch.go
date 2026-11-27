package journal

import (
	"context"
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"
)

// ClassifyAbandonedDispatches is called by a fresh manager before publishing
// admission. Exclusive journal ownership proves the former dispatcher is gone;
// it does not prove termination of the external harness.
func (j *Journal) ClassifyAbandonedDispatches(ctx context.Context, sessionID, incarnationID, streamID string, generation uint64) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	db, err := j.dbLocked()
	if err != nil {
		return err
	}
	abandoned, writeBytes, err := abandonedJournalDispatches(ctx, db, sessionID, incarnationID, streamID, generation)
	if err != nil || len(abandoned) == 0 {
		return err
	}
	reservation, err := j.checkDiskCapacity(ctx, db, writeBytes, false)
	if err != nil {
		return err
	}
	err = j.updateLocked(func(tx *bolt.Tx) error {
		for _, submission := range abandoned {
			if _, err := j.transitionSubmissionTx(ctx, tx, submission.ID, SubmissionInterruptedUnknown, time.Now().UTC()); err != nil {
				return err
			}
		}
		return nil
	})
	j.finishDiskCapacityReservation(reservation, err == nil)
	recordSubmissionWriteError(err)
	return err
}

func abandonedJournalDispatches(ctx context.Context, db *bolt.DB, sessionID, incarnationID, streamID string, generation uint64) ([]Submission, int64, error) {
	var abandoned []Submission
	var writeBytes int64
	err := db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSubmissions).ForEach(func(_, raw []byte) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			var submission Submission
			if raw == nil || json.Unmarshal(raw, &submission) != nil {
				return ErrJournalCorrupt
			}
			if !submissionNeedsRecovery(submission) {
				return nil
			}
			if !abandonedSubmissionOwnerMatches(tx, submission, sessionID, incarnationID, streamID, generation) {
				return ErrOwnerMismatch
			}
			if submission.State == SubmissionDispatching {
				abandoned = append(abandoned, submission)
				writeBytes = requiredDiskBytes(writeBytes, int64(len(raw))+256, 0)
			}
			return nil
		})
	})
	return abandoned, writeBytes, err
}

func abandonedSubmissionOwnerMatches(tx *bolt.Tx, submission Submission, sessionID, incarnationID, streamID string, generation uint64) bool {
	return sessionID != "" && incarnationID != "" && streamID != "" && generation > 0 &&
		submission.SessionID == sessionID && submission.IncarnationID == incarnationID &&
		submission.HarnessGeneration > 0 && submission.HarnessGeneration <= generation &&
		abandonedSubmissionStreamMatches(tx, submission, streamID, generation) &&
		submission.ID != "" && submission.Hash != ""
}

func abandonedSubmissionStreamMatches(tx *bolt.Tx, submission Submission, streamID string, generation uint64) bool {
	if submission.StreamID == "" {
		return false
	}
	if submission.StreamID == streamID {
		return true
	}
	if submission.HarnessGeneration >= generation {
		return false
	}
	if submission.State == SubmissionInterruptedUnknown {
		return true
	}
	stream, err := decodeStream(tx.Bucket(bucketStreamMeta).Get([]byte(submission.StreamID)))
	return err == nil && stream.StreamID == submission.StreamID && stream.SessionID == submission.SessionID &&
		stream.IncarnationID == submission.IncarnationID && stream.HarnessGeneration == submission.HarnessGeneration
}
