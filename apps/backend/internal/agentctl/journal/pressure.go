package journal

import (
	"context"

	bolt "go.etcd.io/bbolt"
)

// Capacity separates logical retention from physical bbolt file allocation.
// Guarded reserves room for bounded producer writes and terminal evidence.
type Capacity struct {
	StreamBytes  int64 `json:"stream_bytes"`
	StreamLimit  int64 `json:"stream_limit"`
	JournalBytes int64 `json:"journal_bytes"`
	JournalLimit int64 `json:"journal_limit"`
	Pressure     bool  `json:"pressure"`
	Guarded      bool  `json:"guarded"`
	Recovered    bool  `json:"recovered"`
}

func (j *Journal) Capacity(ctx context.Context, streamID string) (Capacity, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	db, err := j.dbLocked()
	if err != nil {
		return Capacity{}, err
	}
	result := Capacity{StreamLimit: j.config.MaxStreamBytes - min(j.config.ReserveBytes, j.config.MaxStreamBytes/10), JournalLimit: j.config.MaxJournalBytes - j.config.ReserveBytes}
	err = db.View(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		stream, err := decodeStream(tx.Bucket(bucketStreamMeta).Get([]byte(streamID)))
		if err != nil {
			return err
		}
		result.StreamBytes = stream.Bytes
		result.JournalBytes, err = decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
		return err
	})
	if err != nil {
		return Capacity{}, err
	}
	result.Pressure = capacityAt(result.StreamBytes, result.StreamLimit, 75) || capacityAt(result.JournalBytes, result.JournalLimit, 75)
	// Account for the queue, one maximum serialized event, and terminal reserve.
	headroom := int64(4<<20) + j.config.MaxEventBytes*2 + j.config.ReserveBytes
	streamGuard := min(result.StreamLimit*90/100, max(result.StreamLimit*60/100, result.StreamLimit-headroom))
	journalGuard := min(result.JournalLimit*90/100, max(result.JournalLimit*60/100, result.JournalLimit-headroom))
	result.Guarded = result.StreamBytes >= streamGuard || result.JournalBytes >= journalGuard
	result.Recovered = !capacityAt(result.StreamBytes, result.StreamLimit, 60) && !capacityAt(result.JournalBytes, result.JournalLimit, 60)
	return result, nil
}

func capacityAt(bytes, limit, percent int64) bool { return limit <= 0 || bytes >= limit*percent/100 }
