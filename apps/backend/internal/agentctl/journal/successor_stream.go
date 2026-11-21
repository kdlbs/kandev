package journal

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	bolt "go.etcd.io/bbolt"
)

// ListSuccessorStreams returns streams owned by the same session lifetime and
// created after the caller's configured generation. Event payloads are not
// included in the inventory.
func (j *Journal) ListSuccessorStreams(ctx context.Context, sessionID, incarnationID string, ownerGeneration uint64) ([]Stream, error) {
	if sessionID == "" || incarnationID == "" || ownerGeneration == 0 {
		return nil, ErrOwnerMismatch
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	db, err := j.dbLocked()
	if err != nil {
		return nil, err
	}
	var result []Stream
	err = db.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return tx.Bucket(bucketStreamMeta).ForEach(func(_, raw []byte) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if raw == nil {
				return nil
			}
			stream, err := decodeStream(raw)
			if err != nil {
				return err
			}
			if stream.SessionID == sessionID && stream.IncarnationID == incarnationID &&
				stream.HarnessGeneration > ownerGeneration {
				result = append(result, stream)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, k int) bool {
		if result[i].HarnessGeneration == result[k].HarnessGeneration {
			return result[i].StreamID < result[k].StreamID
		}
		return result[i].HarnessGeneration < result[k].HarnessGeneration
	})
	return result, nil
}

// AbandonStream removes an uncommitted successor stream so its deterministic
// stream ID can be used by a later start. The expected snapshot prevents a
// stale inventory from discarding events appended after it was read.
func (j *Journal) AbandonStream(ctx context.Context, expected Stream) error {
	if expected.StreamID == "" || expected.SessionID == "" || expected.IncarnationID == "" ||
		expected.HarnessGeneration == 0 || expected.HighWater <= expected.Acknowledged {
		return ErrOwnerMismatch
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	if _, err := j.dbLocked(); err != nil {
		RecordJournalError(classifyJournalError(err))
		return err
	}
	err := j.updateLocked(func(tx *bolt.Tx) error { return abandonStreamTx(ctx, tx, expected) })
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			RecordJournalError(classifyJournalError(err))
		}
		return err
	}
	j.refreshMetricsLocked()
	return nil
}

func abandonStreamTx(ctx context.Context, tx *bolt.Tx, expected Stream) error {
	stream, err := validateStreamAbandonmentTx(ctx, tx, expected)
	if err != nil {
		return err
	}
	removedBytes, err := deleteAbandonedStreamEventsTx(tx, stream)
	if err != nil {
		return err
	}
	journalBytes, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
	if err != nil {
		return err
	}
	if removedBytes > journalBytes {
		return ErrJournalCorrupt
	}
	return removeAbandonedStreamTx(tx, stream, journalBytes-removedBytes)
}

func validateStreamAbandonmentTx(ctx context.Context, tx *bolt.Tx, expected Stream) (Stream, error) {
	if err := ctx.Err(); err != nil {
		return Stream{}, err
	}
	stream, err := decodeStream(tx.Bucket(bucketStreamMeta).Get([]byte(expected.StreamID)))
	if err != nil {
		return Stream{}, err
	}
	if stream.StreamID == "" {
		return Stream{}, ErrStreamNotFound
	}
	if stream.SessionID != expected.SessionID || stream.IncarnationID != expected.IncarnationID ||
		stream.HarnessGeneration != expected.HarnessGeneration {
		return Stream{}, ErrOwnerMismatch
	}
	if stream.HighWater != expected.HighWater || stream.Acknowledged != expected.Acknowledged || stream.Sealed ||
		stream.HighWater <= stream.Acknowledged {
		return Stream{}, ErrSequenceConflict
	}
	busy, err := streamHasUnsettledSubmissions(tx, stream)
	if err != nil {
		return Stream{}, err
	}
	if busy {
		return Stream{}, ErrSubmissionState
	}
	return stream, nil
}

func deleteAbandonedStreamEventsTx(tx *bolt.Tx, stream Stream) (int64, error) {
	events := tx.Bucket(bucketEvents)
	bucket := events.Bucket([]byte(stream.StreamID))
	if bucket == nil {
		if stream.Bytes != 0 {
			return 0, ErrJournalCorrupt
		}
		return 0, nil
	}
	var removedBytes int64
	if err := bucket.ForEach(func(_, raw []byte) error {
		if raw == nil {
			return nil
		}
		var event Event
		if err := json.Unmarshal(raw, &event); err != nil {
			return ErrJournalCorrupt
		}
		if event.StreamID != stream.StreamID || event.SessionID != stream.SessionID ||
			event.IncarnationID != stream.IncarnationID || event.HarnessGeneration != stream.HarnessGeneration {
			return ErrOwnerMismatch
		}
		removedBytes += int64(len(raw))
		return nil
	}); err != nil {
		return 0, err
	}
	if removedBytes != stream.Bytes {
		return 0, ErrJournalCorrupt
	}
	if err := events.DeleteBucket([]byte(stream.StreamID)); err != nil {
		return 0, err
	}
	return removedBytes, nil
}

func removeAbandonedStreamTx(tx *bolt.Tx, stream Stream, journalBytes int64) error {
	if err := deleteAbandonedStreamIdentitiesTx(tx, stream.StreamID); err != nil {
		return err
	}
	if err := tx.Bucket(bucketStreamMeta).Delete([]byte(stream.StreamID)); err != nil {
		return err
	}
	return tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(journalBytes))
}

func deleteAbandonedStreamIdentitiesTx(tx *bolt.Tx, streamID string) error {
	identities := tx.Bucket(bucketEventIdentities)
	if identities == nil {
		return nil
	}
	var keys [][]byte
	if err := identities.ForEach(func(key, raw []byte) error {
		if raw == nil {
			return nil
		}
		var receipt eventIdentityReceipt
		if err := json.Unmarshal(raw, &receipt); err != nil {
			return ErrJournalCorrupt
		}
		if receipt.StreamID == streamID {
			keys = append(keys, append([]byte(nil), key...))
		}
		return nil
	}); err != nil {
		return err
	}
	for _, key := range keys {
		if err := identities.Delete(key); err != nil {
			return err
		}
	}
	return nil
}
