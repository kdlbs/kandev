package journal

import (
	"bytes"
	"context"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

// CompareAndWriteInferenceOwnerState commits a closure transition only while
// the complete observed state remains unchanged, including same-owner writes.
func (j *Journal) CompareAndWriteInferenceOwnerState(ctx context.Context, expected, data []byte, terminal bool) error {
	j.mu.RLock()
	defer j.mu.RUnlock()
	db, err := j.dbLocked()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if int64(len(data)) > j.config.MaxEventBytes {
		return fmt.Errorf("inference ownership metadata exceeds journal record limit")
	}
	reservation, err := j.checkDiskCapacity(ctx, db, int64(len(data)), !terminal)
	if err != nil {
		return err
	}
	err = j.updateLocked(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bucketMeta)
		if meta == nil {
			return ErrJournalCorrupt
		}
		if !bytes.Equal(meta.Get(keyInferenceOwner), expected) {
			return ErrInferenceOwnerConflict
		}
		return meta.Put(keyInferenceOwner, data)
	})
	j.finishDiskCapacityReservation(reservation, err == nil)
	return err
}
