package journal

import (
	"context"
	"encoding/json"
	"errors"

	bolt "go.etcd.io/bbolt"
)

// RolloverStream seals an idle transport stream and creates its replacement
// under the same authenticated owner. It does not alter harness generation or
// native conversation identity. Old submission tombstones remain in place and
// therefore cannot become dispatchable after payload pruning.
func (j *Journal) RolloverStream(ctx context.Context, oldStreamID string, replacement Stream) error {
	if j == nil {
		return errors.New("journal is nil")
	}
	if oldStreamID == "" || replacement.StreamID == "" || oldStreamID == replacement.StreamID {
		return ErrOwnerMismatch
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	err := j.db.Update(func(tx *bolt.Tx) error {
		old, err := validateRolloverStreamTx(ctx, tx, oldStreamID, replacement)
		if err != nil {
			return err
		}
		return writeRolloverStreamsTx(tx, oldStreamID, old, replacement)
	})
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			RecordJournalError(classifyJournalError(err))
		}
	}
	return err
}

func validateRolloverStreamTx(ctx context.Context, tx *bolt.Tx, oldStreamID string, replacement Stream) (Stream, error) {
	if err := ctx.Err(); err != nil {
		return Stream{}, err
	}
	streams := tx.Bucket(bucketStreamMeta)
	old, err := decodeStream(streams.Get([]byte(oldStreamID)))
	if err != nil {
		return Stream{}, err
	}
	if old.StreamID == "" {
		return Stream{}, ErrStreamNotFound
	}
	if old.Sealed || old.HighWater != old.Acknowledged {
		return Stream{}, ErrSubmissionState
	}
	if !sameRolloverOwner(old, replacement) {
		return Stream{}, ErrOwnerMismatch
	}
	existing, err := decodeStream(streams.Get([]byte(replacement.StreamID)))
	if err != nil {
		return Stream{}, err
	}
	if existing.StreamID != "" {
		return Stream{}, ErrSequenceConflict
	}
	busy, err := streamHasUnsettledSubmissions(tx, old)
	if err != nil {
		return Stream{}, err
	}
	if busy {
		return Stream{}, ErrSubmissionState
	}
	return old, nil
}

func sameRolloverOwner(old, replacement Stream) bool {
	return (replacement.SessionID == "" || replacement.SessionID == old.SessionID) &&
		(replacement.IncarnationID == "" || replacement.IncarnationID == old.IncarnationID) &&
		(replacement.HarnessGeneration == 0 || replacement.HarnessGeneration == old.HarnessGeneration)
}

func writeRolloverStreamsTx(tx *bolt.Tx, oldStreamID string, old, replacement Stream) error {
	streams := tx.Bucket(bucketStreamMeta)
	old.Sealed = true
	encodedOld, err := json.Marshal(old)
	if err != nil {
		return err
	}
	if err := streams.Put([]byte(oldStreamID), encodedOld); err != nil {
		return err
	}
	replacement.SessionID = old.SessionID
	replacement.IncarnationID = old.IncarnationID
	replacement.HarnessGeneration = old.HarnessGeneration
	replacement.HighWater = 0
	replacement.Acknowledged = 0
	replacement.FirstRetained = 0
	replacement.Bytes = 0
	replacement.Sealed = false
	encodedReplacement, err := json.Marshal(replacement)
	if err != nil {
		return err
	}
	return streams.Put([]byte(replacement.StreamID), encodedReplacement)
}

func streamHasUnsettledSubmissions(tx *bolt.Tx, stream Stream) (bool, error) {
	var busy bool
	err := tx.Bucket(bucketSubmissions).ForEach(func(_, raw []byte) error {
		if raw == nil {
			return nil
		}
		var submission Submission
		if err := json.Unmarshal(raw, &submission); err != nil {
			return ErrJournalCorrupt
		}
		if submission.StreamID != stream.StreamID || submission.Retired {
			return nil
		}
		switch submission.State {
		case SubmissionCompleted, SubmissionFailed, SubmissionCancelled:
			if submission.State == SubmissionCompleted && !submission.TerminalEventRetained {
				busy = true
			}
		default:
			busy = true
		}
		return nil
	})
	return busy, err
}
