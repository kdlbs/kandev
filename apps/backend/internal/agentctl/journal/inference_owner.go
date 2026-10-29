package journal

import (
	"context"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

var keyInferenceOwner = []byte("inference_owner")

// Fresh reports initialization of this owner journal, before any child starts.
// An existing journal without inference metadata is not a fresh inventory.
func (j *Journal) Fresh() bool { return j.fresh }

// ReadInferenceOwner reads process ownership metadata from the existing journal.
func (j *Journal) ReadInferenceOwner() ([]byte, bool, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	db, err := j.dbLocked()
	if err != nil {
		return nil, false, err
	}
	var data []byte
	err = db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bucketMeta)
		if meta == nil {
			return ErrJournalCorrupt
		}
		data = append(data, meta.Get(keyInferenceOwner)...)
		return nil
	})
	return data, len(data) != 0, err
}

// WriteInferenceOwner shares the journal's transaction, disk and close boundary.
func (j *Journal) WriteInferenceOwner(ctx context.Context, data []byte, terminal bool) error {
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
		return meta.Put(keyInferenceOwner, data)
	})
	j.finishDiskCapacityReservation(reservation, err == nil)
	return err
}
