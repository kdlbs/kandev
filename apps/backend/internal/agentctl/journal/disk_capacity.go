package journal

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

func (j *Journal) sampleDiskCapacity(ctx context.Context) (int64, bool, error) {
	if err := ctx.Err(); err != nil {
		return -1, false, err
	}
	j.diskCapacityMu.Lock()
	defer j.diskCapacityMu.Unlock()

	now := time.Now()
	if !j.diskCapacityAt.IsZero() && now.Sub(j.diskCapacityAt) < j.config.DiskCapacityCacheTTL {
		return j.diskAvailable, j.diskCapacityKnown, nil
	}
	sampleCtx, cancel := context.WithTimeout(ctx, j.config.DiskCapacityTimeout)
	capacity, err := j.config.DiskCapacityReader(sampleCtx, filepath.Dir(j.config.Path))
	cancel()
	j.diskCapacityAt = time.Now()
	if err != nil || capacity.TotalBytes == 0 {
		j.diskAvailable = -1
		j.diskCapacityKnown = false
		if ctxErr := ctx.Err(); ctxErr != nil {
			return -1, false, ctxErr
		}
		return -1, false, nil
	}
	if capacity.AvailableBytes > capacity.TotalBytes {
		capacity.AvailableBytes = capacity.TotalBytes
	}
	if capacity.AvailableBytes > math.MaxInt64 {
		j.diskAvailable = math.MaxInt64
	} else {
		j.diskAvailable = int64(capacity.AvailableBytes)
	}
	j.diskWrittenBytes = 0
	j.diskCapacityKnown = true
	return j.diskAvailable, true, nil
}

func (j *Journal) effectiveDiskCapacity(ctx context.Context) (int64, bool, error) {
	_, known, err := j.sampleDiskCapacity(ctx)
	if err != nil || !known {
		return -1, known, err
	}
	j.diskCapacityMu.Lock()
	defer j.diskCapacityMu.Unlock()
	if !j.diskCapacityKnown {
		return -1, false, nil
	}
	return j.diskAvailableAfterWritesLocked(), true, nil
}

type diskCapacityReservation struct {
	tracked       bool
	diskBytes     int64
	reusableBytes int64
	fileSize      int64
}

func (j *Journal) checkDiskCapacity(ctx context.Context, db *bolt.DB, writeBytes int64, preserveReserve bool) (diskCapacityReservation, error) {
	_, known, err := j.sampleDiskCapacity(ctx)
	if err != nil {
		return diskCapacityReservation{}, err
	}
	if !known {
		return diskCapacityReservation{}, nil
	}
	fileInfo, statErr := os.Stat(j.config.Path)
	if statErr != nil {
		return diskCapacityReservation{}, statErr
	}
	return j.reserveDiskCapacity(db, writeBytes, preserveReserve, fileInfo.Size())
}

func (j *Journal) reserveDiskCapacity(db *bolt.DB, writeBytes int64, preserveReserve bool, fileSize int64) (diskCapacityReservation, error) {
	j.diskCapacityMu.Lock()
	defer j.diskCapacityMu.Unlock()
	if !j.diskCapacityKnown {
		return diskCapacityReservation{}, nil
	}
	reserve := int64(0)
	if preserveReserve {
		reserve = j.config.DiskReserveBytes
	}
	reusableBytes := reusableJournalPageBytes(db)
	if reusableBytes <= j.diskReusablePendingBytes {
		reusableBytes = 0
	} else {
		reusableBytes -= j.diskReusablePendingBytes
	}
	maxCredit := maxReusableWriteBytes(j.config.MaxEventBytes)
	reuseCredit := min(writeBytes, min(reusableBytes, maxCredit))
	diskBytes := writeBytes - reuseCredit
	required := requiredDiskBytes(reserve, j.config.DiskWriteHeadroomBytes, diskBytes)
	available := j.diskAvailableAfterWritesLocked()
	if required == math.MaxInt64 || available < required {
		return diskCapacityReservation{}, ErrJournalFull
	}
	pendingDisk := requiredDiskBytes(j.diskPendingBytes, diskBytes, 0)
	pendingReusable := requiredDiskBytes(j.diskReusablePendingBytes, reuseCredit, 0)
	if pendingDisk == math.MaxInt64 || pendingReusable == math.MaxInt64 {
		return diskCapacityReservation{}, ErrJournalFull
	}
	j.diskPendingBytes = pendingDisk
	j.diskReusablePendingBytes = pendingReusable
	return diskCapacityReservation{
		tracked:       true,
		diskBytes:     diskBytes,
		reusableBytes: reuseCredit,
		fileSize:      fileSize,
	}, nil
}

func (j *Journal) diskAvailableAfterWritesLocked() int64 {
	charged := requiredDiskBytes(j.diskWrittenBytes, j.diskPendingBytes, 0)
	if charged >= j.diskAvailable {
		return 0
	}
	return j.diskAvailable - charged
}

func (j *Journal) reusableJournalBytes(db *bolt.DB) int64 {
	j.diskCapacityMu.Lock()
	defer j.diskCapacityMu.Unlock()
	reusable := reusableJournalPageBytes(db)
	if reusable <= j.diskReusablePendingBytes {
		return 0
	}
	return reusable - j.diskReusablePendingBytes
}

func reusableJournalPageBytes(db *bolt.DB) int64 {
	pageSize := int64(db.Info().PageSize)
	freePages := int64(db.Stats().FreePageN)
	if pageSize <= 0 || freePages <= 0 || freePages > math.MaxInt64/pageSize {
		return 0
	}
	return freePages * pageSize
}

func (j *Journal) finishDiskCapacityReservation(reservation diskCapacityReservation, committed bool) {
	if !reservation.tracked {
		return
	}
	fileGrowthBytes := int64(0)
	if fileInfo, err := os.Stat(j.config.Path); err == nil && fileInfo.Size() > reservation.fileSize {
		fileGrowthBytes = fileInfo.Size() - reservation.fileSize
	}
	committedDiskBytes := fileGrowthBytes
	if committed {
		committedDiskBytes = max(committedDiskBytes, reservation.diskBytes)
	}
	j.diskCapacityMu.Lock()
	defer j.diskCapacityMu.Unlock()
	j.diskPendingBytes -= reservation.diskBytes
	if j.diskPendingBytes < 0 {
		j.diskPendingBytes = 0
	}
	j.diskReusablePendingBytes -= reservation.reusableBytes
	if j.diskReusablePendingBytes < 0 {
		j.diskReusablePendingBytes = 0
	}
	if committedDiskBytes > 0 {
		j.diskWrittenBytes = requiredDiskBytes(j.diskWrittenBytes, committedDiskBytes, 0)
	}
}

func diskCapacityGuarded(available, reusableBytes, reserveBytes, headroomBytes, maxEventBytes int64) bool {
	maxInt64 := int64(math.MaxInt64)
	maxWriteBytes := maxReusableWriteBytes(maxEventBytes)
	reusableCredit := min(maxWriteBytes, reusableBytes)
	writeHeadroom := requiredDiskBytes(headroomBytes, maxWriteBytes-reusableCredit, 0)
	required := requiredDiskBytes(reserveBytes, writeHeadroom, 0)
	return available < required || required == maxInt64
}

func maxReusableWriteBytes(maxEventBytes int64) int64 {
	return requiredDiskBytes(maxEventBytes, maxEventBytes, maxEventBytes/16)
}

func requiredDiskBytes(first, second, third int64) int64 {
	maxInt64 := int64(math.MaxInt64)
	if first < 0 || second < 0 || third < 0 || first > maxInt64-second || first+second > maxInt64-third {
		return maxInt64
	}
	return first + second + third
}
