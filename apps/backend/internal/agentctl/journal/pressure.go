package journal

import (
	"context"

	bolt "go.etcd.io/bbolt"
)

// Capacity separates logical retention from physical bbolt file allocation.
// Guarded reserves room for bounded producer writes and terminal evidence.
type Capacity struct {
	StreamBytes        int64 `json:"stream_bytes"`
	StreamLimit        int64 `json:"stream_limit"`
	JournalBytes       int64 `json:"journal_bytes"`
	JournalLimit       int64 `json:"journal_limit"`
	DiskAvailableBytes int64 `json:"disk_available_bytes"`
	// JournalReusableBytes reports aggregate bbolt free pages separately from filesystem capacity.
	JournalReusableBytes int64 `json:"journal_reusable_bytes"`
	DiskReserveBytes     int64 `json:"disk_reserve_bytes"`
	DiskCapacityKnown    bool  `json:"disk_capacity_known"`
	Pressure             bool  `json:"pressure"`
	Guarded              bool  `json:"guarded"`
	Recovered            bool  `json:"recovered"`
}

func (j *Journal) Capacity(ctx context.Context, streamID string) (Capacity, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	db, err := j.dbLocked()
	if err != nil {
		return Capacity{}, err
	}
	result := Capacity{
		StreamLimit:        logicalLimit(j.config.MaxStreamBytes, j.config.ReserveBytes),
		JournalLimit:       logicalLimit(j.config.MaxJournalBytes, j.config.ReserveBytes),
		DiskAvailableBytes: -1,
		DiskReserveBytes:   j.config.DiskReserveBytes,
	}
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
	result.DiskAvailableBytes, result.DiskCapacityKnown, err = j.effectiveDiskCapacity(ctx)
	if err != nil {
		return Capacity{}, err
	}
	result.JournalReusableBytes = j.reusableJournalBytes(db)
	logicalPressure := capacityAt(result.StreamBytes, result.StreamLimit, 75) || capacityAt(result.JournalBytes, result.JournalLimit, 75)
	logicalGuard := logicalCapacityGuarded(result.StreamBytes, result.StreamLimit, j.capacityHeadroom()) ||
		logicalCapacityGuarded(result.JournalBytes, result.JournalLimit, j.capacityHeadroom())
	physicalGuard := result.DiskCapacityKnown && diskCapacityGuarded(
		result.DiskAvailableBytes,
		result.JournalReusableBytes,
		j.config.DiskReserveBytes,
		j.config.DiskWriteHeadroomBytes,
		j.config.MaxEventBytes,
	)
	result.Pressure = logicalPressure || physicalGuard
	result.Guarded = logicalGuard || physicalGuard
	result.Recovered = !capacityAt(result.StreamBytes, result.StreamLimit, 60) &&
		!capacityAt(result.JournalBytes, result.JournalLimit, 60) && !physicalGuard
	return result, nil
}

func (j *Journal) capacityHeadroom() int64 {
	return requiredDiskBytes(j.config.DiskWriteHeadroomBytes, j.config.MaxEventBytes, j.config.ReserveBytes)
}

func logicalCapacityGuarded(bytes, limit, headroom int64) bool {
	if limit <= 0 {
		return false
	}
	guard := min(percentOf(limit, 90), max(percentOf(limit, 60), limit-headroom))
	return bytes >= guard
}

func capacityAt(bytes, limit, percent int64) bool {
	return limit > 0 && bytes >= percentOf(limit, percent)
}

func percentOf(value, percent int64) int64 {
	return value/100*percent + value%100*percent/100
}
