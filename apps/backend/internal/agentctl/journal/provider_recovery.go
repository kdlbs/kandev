package journal

import (
	"context"
	"encoding/binary"

	bolt "go.etcd.io/bbolt"
)

// The base format remains compatible until an irreversible provider transition.
// Older helpers reject version three instead of ignoring scoped authority.
const providerMCPJournalVersion uint32 = 3

var keyLatestProviderReplacement = []byte("latest")

type ProviderReplacementRecovery struct {
	Required bool
	Latest   ProviderReplacementReceipt
	Active   ProviderReplacementReceipt
}

func (j *Journal) ProviderReplacementRecovery(ctx context.Context) (ProviderReplacementRecovery, error) {
	var state ProviderReplacementRecovery
	if j == nil {
		return state, ErrJournalClosed
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	err := j.viewLocked(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := providerReceiptAccounting(tx); err != nil {
			return err
		}
		state.Required = binary.BigEndian.Uint32(tx.Bucket(bucketMeta).Get(keyVersion)) >= providerMCPJournalVersion
		bucket := tx.Bucket(bucketProviderReplacements)
		if bucket == nil {
			return nil
		}
		var err error
		state.Latest, err = readProviderReceiptTx(tx, string(bucket.Get(keyLatestProviderReplacement)))
		if err != nil {
			return err
		}
		state.Active, err = readProviderReceiptTx(tx, string(bucket.Get(keyActiveProviderReplacement)))
		return err
	})
	return state, err
}

func recordProviderReaderRequirement(tx *bolt.Tx, bucket *bolt.Bucket, next ProviderReplacementReceipt) error {
	if next.Phase == ProviderReplacementPrepared || next.Phase == ProviderReplacementReleased {
		return nil
	}
	if err := bucket.Put(keyLatestProviderReplacement, []byte(next.OperationID)); err != nil {
		return err
	}
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], providerMCPJournalVersion)
	return tx.Bucket(bucketMeta).Put(keyVersion, encoded[:])
}

func validateProviderReaderRequirement(tx *bolt.Tx, version uint32) error {
	bucket := tx.Bucket(bucketProviderReplacements)
	if version < providerMCPJournalVersion {
		if bucket != nil && len(bucket.Get(keyLatestProviderReplacement)) != 0 {
			return ErrJournalCorrupt
		}
		return nil
	}
	if bucket == nil {
		return ErrJournalCorrupt
	}
	id := string(bucket.Get(keyLatestProviderReplacement))
	if id == "" {
		return ErrJournalCorrupt
	}
	receipt, err := readProviderReceiptTx(tx, id)
	if err != nil {
		return err
	}
	if receipt.OperationID == "" || receipt.Phase == ProviderReplacementPrepared || receipt.Phase == ProviderReplacementReleased {
		return ErrJournalCorrupt
	}
	return nil
}
