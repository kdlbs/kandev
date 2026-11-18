package journal

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"

	bolt "go.etcd.io/bbolt"
)

// providerReceiptAccounting validates the operation index before journal
// admission. Missing ownership evidence cannot make a pending operation idle.
func providerReceiptAccounting(tx *bolt.Tx) (int64, error) {
	bucket := tx.Bucket(bucketProviderReplacements)
	if bucket == nil {
		return 0, nil
	}
	if err := validateProviderReaderRequirement(tx, binary.BigEndian.Uint32(tx.Bucket(bucketMeta).Get(keyVersion))); err != nil {
		return 0, err
	}
	active := string(bucket.Get(keyActiveProviderReplacement))
	var total int64
	var pending int
	err := bucket.ForEach(func(key, raw []byte) error {
		if bytes.Equal(key, keyActiveProviderReplacement) || bytes.Equal(key, keyLatestProviderReplacement) {
			if raw == nil {
				return ErrJournalCorrupt
			}
			return nil
		}
		if !validProviderReceiptEntry(key, raw) {
			return ErrJournalCorrupt
		}
		receipt, err := readProviderReceiptTx(tx, strings.TrimPrefix(string(key), "operation/"))
		if err != nil {
			return err
		}
		if !providerReceiptTerminal(receipt.Phase) {
			pending++
			if active != receipt.OperationID {
				return ErrJournalCorrupt
			}
		}
		total += int64(len(raw))
		return nil
	})
	if err != nil {
		return 0, err
	}
	if pending > 1 || (pending == 0 && active != "") {
		return 0, ErrJournalCorrupt
	}
	return total, nil
}

func (j *Journal) ActiveProviderReplacement(ctx context.Context) (ProviderReplacementReceipt, error) {
	var receipt ProviderReplacementReceipt
	if j == nil {
		return receipt, ErrJournalClosed
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
		bucket := tx.Bucket(bucketProviderReplacements)
		if bucket == nil {
			return nil
		}
		id := string(bucket.Get(keyActiveProviderReplacement))
		if id == "" {
			return nil
		}
		var err error
		receipt, err = readProviderReceiptTx(tx, id)
		return err
	})
	return receipt, err
}

func validProviderReceiptEntry(key, raw []byte) bool {
	return raw != nil && strings.HasPrefix(string(key), "operation/") && len(raw) <= 64<<10
}
