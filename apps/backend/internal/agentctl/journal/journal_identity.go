package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	maxEventIdentityBytes = 256
	maxEventIdentities    = 4096
)

type eventIdentityReceipt struct {
	StreamID    string    `json:"stream_id"`
	Type        string    `json:"type"`
	PayloadHash string    `json:"payload_hash"`
	Sequence    uint64    `json:"sequence"`
	CreatedAt   time.Time `json:"created_at"`
}

func eventPayloadHash(event Event) string {
	sum := sha256.New()
	sum.Write([]byte(event.Type))
	sum.Write([]byte{0})
	sum.Write(event.Payload)
	return hex.EncodeToString(sum.Sum(nil))
}

func lookupEventIdentityTx(tx *bolt.Tx, identity string) (eventIdentityReceipt, bool, error) {
	bucket, err := tx.CreateBucketIfNotExists(bucketEventIdentities)
	if err != nil {
		return eventIdentityReceipt{}, false, err
	}
	raw := bucket.Get([]byte(identity))
	if raw == nil {
		return eventIdentityReceipt{}, false, nil
	}
	var receipt eventIdentityReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return eventIdentityReceipt{}, false, ErrJournalCorrupt
	}
	return receipt, true, nil
}

func putEventIdentityTx(tx *bolt.Tx, event Event) error {
	bucket, err := tx.CreateBucketIfNotExists(bucketEventIdentities)
	if err != nil {
		return err
	}
	receipt := eventIdentityReceipt{
		StreamID:    event.StreamID,
		Type:        event.Type,
		PayloadHash: eventPayloadHash(event),
		Sequence:    event.Sequence,
		CreatedAt:   event.CreatedAt,
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	if err := bucket.Put([]byte(event.Identity), raw); err != nil {
		return err
	}
	if bucket.Stats().KeyN > maxEventIdentities {
		return pruneEventIdentitiesTx(bucket)
	}
	return nil
}

func pruneEventIdentitiesTx(bucket *bolt.Bucket) error {
	type entry struct {
		key     []byte
		created time.Time
	}
	entries := make([]entry, 0, bucket.Stats().KeyN)
	cursor := bucket.Cursor()
	for key, raw := cursor.First(); key != nil; key, raw = cursor.Next() {
		var receipt eventIdentityReceipt
		if err := json.Unmarshal(raw, &receipt); err != nil {
			return ErrJournalCorrupt
		}
		entries = append(entries, entry{key: append([]byte(nil), key...), created: receipt.CreatedAt})
	}
	remove := len(entries) / 4
	if remove == 0 {
		remove = 1
	}
	for index := 0; index < remove; index++ {
		oldest := index
		for candidate := index + 1; candidate < len(entries); candidate++ {
			if entries[candidate].created.Before(entries[oldest].created) {
				oldest = candidate
			}
		}
		entries[index], entries[oldest] = entries[oldest], entries[index]
		if err := bucket.Delete(entries[index].key); err != nil {
			return err
		}
	}
	return nil
}
