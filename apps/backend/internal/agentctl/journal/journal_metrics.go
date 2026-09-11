package journal

import bolt "go.etcd.io/bbolt"

func (j *Journal) refreshMetrics() {
	if j == nil {
		return
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.db == nil {
		return
	}
	j.refreshMetricsLocked()
}

func (j *Journal) refreshMetricsLocked() {
	if j == nil || j.db == nil {
		return
	}
	var journalBytes, unacknowledgedBytes int64
	err := j.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bucketMeta)
		var decodeErr error
		journalBytes, decodeErr = decodeInt64(meta.Get(keyJournalBytes))
		if decodeErr != nil {
			return decodeErr
		}
		return tx.Bucket(bucketStreamMeta).ForEach(func(_, value []byte) error {
			stream, err := decodeStream(value)
			if err != nil {
				return err
			}
			unacknowledgedBytes += stream.Bytes
			return nil
		})
	})
	if err != nil {
		RecordJournalError(classifyJournalError(err))
		return
	}
	SetJournalGauges(journalBytes, unacknowledgedBytes)
}
