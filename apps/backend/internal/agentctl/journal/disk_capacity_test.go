package journal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	systemmetrics "github.com/kandev/kandev/internal/system/metrics"
	bolt "go.etcd.io/bbolt"
)

func TestPhysicalDiskReserveGuardsWritesAndCachesCapacity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	available := uint64(DefaultDiskReserveBytes + DefaultMaxEventBytes)
	var calls atomic.Int64
	reader := func(_ context.Context, gotPath string) (systemmetrics.DiskCapacity, error) {
		calls.Add(1)
		if gotPath != filepath.Dir(path) {
			t.Fatalf("disk capacity path = %q, want journal volume %q", gotPath, filepath.Dir(path))
		}
		return systemmetrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: available}, nil
	}
	j, err := Open(Config{Path: path, DiskCapacityCacheTTL: time.Hour, DiskCapacityReader: reader})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	capacity, err := j.Capacity(context.Background(), "stream")
	if err != nil {
		t.Fatal(err)
	}
	if !capacity.DiskCapacityKnown || !capacity.Guarded {
		t.Fatalf("low disk capacity = %+v, want known guarded pressure", capacity)
	}
	_, err = j.Append(context.Background(), Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Type: "message", Payload: []byte("ordinary output"),
	})
	if !errors.Is(err, ErrJournalFull) {
		t.Fatalf("append below physical reserve error = %v, want journal full", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("capacity samples = %d, want one cached sample", got)
	}
}

func TestCachedDiskCapacityChargesWritesUntilNextSample(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	event := Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Type: "message", Payload: make([]byte, 128<<10), CreatedAt: time.Now().UTC(),
	}
	writeBytes, err := estimateEncodedEventBytes([]Event{event})
	if err != nil {
		t.Fatal(err)
	}
	var available atomic.Uint64
	available.Store(uint64(DefaultDiskReserveBytes + DefaultDiskWriteHeadroomBytes + writeBytes + writeBytes/2))
	var calls atomic.Int64
	j, err := Open(Config{
		Path: path, DiskCapacityCacheTTL: time.Hour,
		DiskCapacityReader: func(context.Context, string) (systemmetrics.DiskCapacity, error) {
			calls.Add(1)
			return systemmetrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: available.Load()}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	initial, err := j.Capacity(context.Background(), "stream")
	if err != nil {
		t.Fatal(err)
	}
	initialFile, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	reuseCredit := min(writeBytes, min(initial.JournalReusableBytes, maxReusableWriteBytes(DefaultMaxEventBytes)))

	if _, err := j.Append(context.Background(), event); err != nil {
		t.Fatalf("first append with cached capacity: %v", err)
	}
	afterFile, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	diskCharge := max(writeBytes-reuseCredit, afterFile.Size()-initialFile.Size())
	capacity, err := j.Capacity(context.Background(), "stream")
	if err != nil {
		t.Fatal(err)
	}
	if diskCharge <= 0 || capacity.DiskAvailableBytes != int64(available.Load())-diskCharge {
		t.Fatalf("effective disk capacity = %d, raw=%d charge=%d; want bounded cached write charge", capacity.DiskAvailableBytes, available.Load(), diskCharge)
	}
	if capacity.DiskAvailableBytes < 0 || capacity.DiskAvailableBytes > int64(available.Load()) {
		t.Fatalf("effective disk capacity = %d, want a nonnegative value no greater than raw free bytes %d", capacity.DiskAvailableBytes, available.Load())
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("disk samples = %d, want one cached sample", got)
	}
	blocked := false
	for attempt := 0; attempt < 8; attempt++ {
		if _, err := j.Append(context.Background(), event); errors.Is(err, ErrJournalFull) {
			blocked = true
			break
		} else if err != nil {
			t.Fatalf("cached append %d error = %v", attempt+2, err)
		}
	}
	if !blocked {
		t.Fatal("cached appends never exhausted usable headroom before a fresh disk sample")
	}
	available.Store(uint64(DefaultDiskReserveBytes + DefaultDiskWriteHeadroomBytes + 4*writeBytes))
	j.diskCapacityMu.Lock()
	j.diskCapacityAt = time.Time{}
	j.diskCapacityMu.Unlock()
	refreshed, err := j.Capacity(context.Background(), "stream")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := refreshed.DiskAvailableBytes, int64(available.Load()); got != want {
		t.Fatalf("refreshed disk capacity = %d, want new OS sample %d", got, want)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("disk samples after capacity refresh = %d, want two", got)
	}
}

func TestDiskReserveIncludesBboltAllocationGrowthAtLargeFileBoundary(t *testing.T) {
	if DefaultDiskAllocationHeadroomBytes < bolt.DefaultAllocSize {
		t.Fatalf("allocation headroom = %d, below bbolt allocation chunk %d", DefaultDiskAllocationHeadroomBytes, bolt.DefaultAllocSize)
	}
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	var available atomic.Uint64
	available.Store(1 << 40)
	j, err := Open(Config{
		Path: path, DiskCapacityCacheTTL: time.Nanosecond,
		DiskCapacityReader: func(context.Context, string) (systemmetrics.DiskCapacity, error) {
			free := available.Load()
			return systemmetrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: free}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	for i := 0; i < 64; i++ {
		if _, err := j.Append(ctx, Event{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
			StreamID: "stream", Type: "message", Payload: make([]byte, 512<<10),
		}); err != nil {
			t.Fatalf("append %d while disk is available: %v", i+1, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > bolt.DefaultAllocSize {
			break
		}
		if i == 63 {
			t.Fatalf("journal file did not cross bbolt allocation boundary: size=%d", info.Size())
		}
	}

	event := Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Type: "message", Payload: []byte("next event near a bbolt allocation boundary"),
	}
	writeBytes, err := estimateEncodedEventBytes([]Event{event})
	if err != nil {
		t.Fatal(err)
	}
	withoutAllocationChunk := DefaultDiskReserveBytes + (DefaultDiskWriteHeadroomBytes - DefaultDiskAllocationHeadroomBytes) + writeBytes
	available.Store(uint64(withoutAllocationChunk))
	time.Sleep(time.Millisecond)
	if _, err := j.Append(ctx, event); !errors.Is(err, ErrJournalFull) {
		t.Fatalf("append with only encoded-write headroom near allocation boundary = %v, want reserve protection", err)
	}
}

func TestMixedBatchCannotSpendReserveForOrdinaryOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	ordinary := Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Type: "message", Payload: []byte("ordinary output"),
	}
	terminal := Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", SubmissionID: "submission", Type: "complete", Terminal: true,
	}
	writeBytes, err := estimateEncodedEventBytes([]Event{ordinary, terminal})
	if err != nil {
		t.Fatal(err)
	}
	var available atomic.Uint64
	available.Store(1 << 40)
	j, err := Open(Config{
		Path: path, DiskCapacityCacheTTL: time.Nanosecond,
		DiskCapacityReader: func(context.Context, string) (systemmetrics.DiskCapacity, error) {
			return systemmetrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: available.Load()}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	if _, err := j.PutSubmission(context.Background(), Submission{
		ID: "submission", StreamID: "stream", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 1, Hash: "hash", State: SubmissionDispatching,
	}); err != nil {
		t.Fatalf("put submission: %v", err)
	}
	available.Store(uint64(DefaultDiskWriteHeadroomBytes + writeBytes + DefaultDiskReserveBytes/2))
	time.Sleep(time.Millisecond)

	if _, err := j.AppendBatch(context.Background(), []Event{ordinary, terminal}); !errors.Is(err, ErrJournalFull) {
		t.Fatalf("mixed ordinary/terminal batch error = %v, want reserve protection", err)
	}
	if _, err := j.Append(context.Background(), terminal); err != nil {
		t.Fatalf("terminal-only write should be allowed to use the reserve: %v", err)
	}
}

func TestSubmissionAdmissionPreservesAndChargesCachedDiskReserve(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	createdAt := time.Date(2026, time.January, 1, 0, 0, 0, 123456789, time.UTC)
	first := Submission{
		ID: "submission-1", Hash: "hash-1", Payload: make([]byte, 64<<10),
		State: SubmissionPrepared, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	writeBytes := int64(len(encoded))
	available := uint64(DefaultDiskReserveBytes + DefaultDiskWriteHeadroomBytes + writeBytes + writeBytes/2)
	var calls atomic.Int64
	j, err := Open(Config{
		Path: path, DiskCapacityCacheTTL: time.Hour,
		DiskCapacityReader: func(context.Context, string) (systemmetrics.DiskCapacity, error) {
			calls.Add(1)
			return systemmetrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: available}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	if _, err := j.PutSubmission(context.Background(), first); err != nil {
		t.Fatalf("first prompt admission: %v", err)
	}
	second := first
	second.ID = "submission-2"
	second.Hash = "hash-2"
	if _, err := j.PutSubmission(context.Background(), second); !errors.Is(err, ErrJournalFull) {
		t.Fatalf("second prompt admission with only cached reserve remaining = %v, want journal full", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("disk samples = %d, want one cached sample", got)
	}
	if duplicate, err := j.PutSubmission(context.Background(), first); err != nil || duplicate.ID != first.ID {
		t.Fatalf("same-hash retry under pressure = %+v, err=%v", duplicate, err)
	}
}

func TestOrdinarySubmissionTransitionPreservesReserveButTerminalCanUseIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	var available atomic.Uint64
	available.Store(1 << 40)
	createdAt := time.Date(2026, time.January, 1, 0, 0, 0, 123456789, time.UTC)
	submission := Submission{
		ID: "submission", Hash: "hash", Payload: []byte("prompt"),
		State: SubmissionPrepared, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	terminalTime := createdAt.Add(time.Minute)
	terminal := submission
	terminal.State = SubmissionCancelled
	terminal.UpdatedAt = terminalTime
	terminalBytes, err := json.Marshal(terminal)
	if err != nil {
		t.Fatal(err)
	}
	j, err := Open(Config{
		Path: path, DiskCapacityCacheTTL: time.Nanosecond,
		DiskCapacityReader: func(context.Context, string) (systemmetrics.DiskCapacity, error) {
			free := available.Load()
			return systemmetrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: free}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	if _, err := j.PutSubmission(context.Background(), submission); err != nil {
		t.Fatalf("put submission: %v", err)
	}
	available.Store(uint64(DefaultDiskWriteHeadroomBytes + int64(len(terminalBytes)) + 1))
	time.Sleep(time.Millisecond)
	if _, err := j.TransitionSubmission(context.Background(), submission.ID, SubmissionAccepted, terminalTime); !errors.Is(err, ErrJournalFull) {
		t.Fatalf("ordinary accepted transition under reserve-only capacity = %v, want journal full", err)
	}
	if _, err := j.TransitionSubmission(context.Background(), submission.ID, SubmissionCancelled, terminalTime); err != nil {
		t.Fatalf("terminal transition should use the reserved capacity: %v", err)
	}
}

func TestCancelSubmissionCanUseReservedTerminalCapacity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	var available atomic.Uint64
	available.Store(1 << 40)
	createdAt := time.Date(2026, time.January, 1, 0, 0, 0, 123456789, time.UTC)
	j, err := Open(Config{
		Path: path, DiskCapacityCacheTTL: time.Nanosecond,
		DiskCapacityReader: func(context.Context, string) (systemmetrics.DiskCapacity, error) {
			free := available.Load()
			return systemmetrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: free}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	submission, err := j.PutSubmission(context.Background(), Submission{
		ID: "submission", StreamID: "stream", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 1, Hash: "hash", Payload: []byte("prompt"),
		State: SubmissionPrepared, CreatedAt: createdAt, UpdatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("put submission: %v", err)
	}
	event := Event{
		SessionID: submission.SessionID, IncarnationID: submission.IncarnationID, HarnessGeneration: 1,
		StreamID: submission.StreamID, SubmissionID: submission.ID, Type: "cancelled", Terminal: true,
		CreatedAt: createdAt.Add(time.Minute),
	}
	writeBytes, err := estimateEncodedEventBytes([]Event{event})
	if err != nil {
		t.Fatal(err)
	}
	available.Store(uint64(DefaultDiskWriteHeadroomBytes + writeBytes + 1))
	time.Sleep(time.Millisecond)
	if err := j.CancelSubmission(context.Background(), submission.ID, event); err != nil {
		t.Fatalf("terminal cancellation should use the reserved capacity: %v", err)
	}
}

func TestDiskCapacityReaderHasADeadlineAndTimeoutIsUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	var calls atomic.Int64
	j, err := Open(Config{
		Path: path, DiskCapacityTimeout: 10 * time.Millisecond,
		DiskCapacityCacheTTL: time.Hour,
		DiskCapacityReader: func(ctx context.Context, _ string) (systemmetrics.DiskCapacity, error) {
			calls.Add(1)
			if _, ok := ctx.Deadline(); !ok {
				t.Error("disk capacity reader has no deadline")
			}
			<-ctx.Done()
			return systemmetrics.DiskCapacity{}, ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	started := time.Now()
	if _, err := j.Append(context.Background(), Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Type: "message", Payload: []byte("unknown capacity remains non-blocking"),
	}); err != nil {
		t.Fatalf("append with timed out capacity sample: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("disk sampling blocked append for %s", elapsed)
	}
	capacity, err := j.Capacity(context.Background(), "stream")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.DiskCapacityKnown || capacity.DiskAvailableBytes != -1 || capacity.Guarded || capacity.Pressure {
		t.Fatalf("timed out disk statistics established pressure: %+v", capacity)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("disk samples = %d, want cached unknown result", got)
	}
}

func TestUnknownDiskCapacityDoesNotBlockJournalOrProveExhaustion(t *testing.T) {
	j, err := Open(Config{
		Path: filepath.Join(t.TempDir(), "delivery.bbolt"),
		DiskCapacityReader: func(context.Context, string) (systemmetrics.DiskCapacity, error) {
			return systemmetrics.DiskCapacity{}, errors.New("capacity unavailable")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	capacity, err := j.Capacity(context.Background(), "stream")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.DiskCapacityKnown || capacity.DiskAvailableBytes != -1 || capacity.Guarded || capacity.Pressure {
		t.Fatalf("unknown disk statistics established pressure: %+v", capacity)
	}
	if _, err := j.Append(context.Background(), Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Type: "message", Payload: []byte("retained without a disk sample"),
	}); err != nil {
		t.Fatalf("append with unknown disk capacity: %v", err)
	}
}

func TestAcknowledgmentSkipsCompactionWithoutCopyHeadroom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	var available atomic.Uint64
	available.Store(1 << 40)
	j, err := Open(Config{
		Path: path, DiskCapacityCacheTTL: time.Nanosecond,
		DiskCapacityReader: func(context.Context, string) (systemmetrics.DiskCapacity, error) {
			free := available.Load()
			return systemmetrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: free}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	var last Event
	for i := 0; i < 6; i++ {
		last, err = j.Append(ctx, Event{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
			StreamID: "stream", Type: "message", Payload: make([]byte, 900_000),
		})
		if err != nil {
			t.Fatalf("append event %d: %v", i+1, err)
		}
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if beforeInfo.Size() <= 4<<20 {
		t.Fatalf("journal file size = %d, want enough reclaimable space to compact", beforeInfo.Size())
	}
	available.Store(uint64(DefaultDiskReserveBytes + DefaultDiskWriteHeadroomBytes))
	if err := j.Acknowledge(ctx, "stream", last.Sequence); err != nil {
		t.Fatalf("acknowledge events under disk pressure: %v", err)
	}
	afterAck, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if afterAck.Size() != beforeInfo.Size() {
		t.Fatalf("ACK rewrote journal from %d to %d bytes without compaction headroom", beforeInfo.Size(), afterAck.Size())
	}
	if err := j.CompactIfNeeded(ctx); err != nil {
		t.Fatalf("skip optional compaction without headroom: %v", err)
	}
	afterCompact, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if afterCompact.Size() != afterAck.Size() {
		t.Fatalf("journal size changed from %d to %d without compaction headroom", afterAck.Size(), afterCompact.Size())
	}
	stream, err := j.GetStream(ctx, "stream")
	if err != nil || stream.Acknowledged != last.Sequence || stream.Bytes != 0 {
		t.Fatalf("ACK pruning under pressure = %+v, err=%v", stream, err)
	}
}

func TestAcknowledgedPagesAllowWriteWhileOSReserveRemainsIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	var available atomic.Uint64
	available.Store(1 << 40)
	j, err := Open(Config{
		Path: path, DiskCapacityCacheTTL: time.Nanosecond,
		DiskCapacityReader: func(context.Context, string) (systemmetrics.DiskCapacity, error) {
			free := available.Load()
			return systemmetrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: free}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	var last Event
	for i := 0; i < 50; i++ {
		last, err = j.Append(ctx, Event{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
			StreamID: "stream", Type: "message", Payload: make([]byte, 512<<10),
		})
		if err != nil {
			t.Fatalf("append %d before acknowledgement: %v", i+1, err)
		}
	}
	beforeAck, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if beforeAck.Size() <= DefaultDiskAllocationHeadroomBytes {
		t.Fatalf("journal size = %d, want a grown bbolt file", beforeAck.Size())
	}

	available.Store(uint64(DefaultDiskReserveBytes + DefaultDiskWriteHeadroomBytes))
	time.Sleep(time.Millisecond)
	if err := j.Acknowledge(ctx, "stream", last.Sequence); err != nil {
		t.Fatalf("acknowledge while OS free space is near reserve: %v", err)
	}
	afterAck, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if afterAck.Size() != beforeAck.Size() {
		t.Fatalf("ACK unexpectedly compacted journal under low disk: %d -> %d", beforeAck.Size(), afterAck.Size())
	}
	db, err := j.dbLocked()
	if err != nil {
		t.Fatal(err)
	}
	reusableBytes := int64(db.Stats().FreePageN) * int64(db.Info().PageSize)
	maxReuseCredit := maxReusableWriteBytes(DefaultMaxEventBytes)
	if reusableBytes < maxReuseCredit {
		t.Fatalf("reusable bbolt pages = %d bytes, want at least %d", reusableBytes, maxReuseCredit)
	}
	capacity, err := j.Capacity(ctx, "stream")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.DiskAvailableBytes != int64(DefaultDiskReserveBytes+DefaultDiskWriteHeadroomBytes) {
		t.Fatalf("reported filesystem capacity = %d, want OS-only free bytes %d", capacity.DiskAvailableBytes, DefaultDiskReserveBytes+DefaultDiskWriteHeadroomBytes)
	}
	if capacity.JournalReusableBytes < maxReuseCredit || capacity.Guarded {
		t.Fatalf("capacity with acknowledged free pages = %+v, want separate reusable pages and no pressure guard", capacity)
	}

	if _, err := j.Append(ctx, Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Type: "message", Payload: []byte("reuses acknowledged bbolt pages"),
	}); err != nil {
		t.Fatalf("append with ACKed reusable pages and OS reserve intact: %v", err)
	}
}

func TestConcurrentWritesCannotDoubleReserveReusablePages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	var available atomic.Uint64
	available.Store(1 << 40)
	j, err := Open(Config{
		Path: path, DiskCapacityCacheTTL: time.Hour,
		DiskCapacityReader: func(context.Context, string) (systemmetrics.DiskCapacity, error) {
			free := available.Load()
			return systemmetrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: free}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	ctx := context.Background()
	var last Event
	for i := 0; i < 16; i++ {
		last, err = j.Append(ctx, Event{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
			StreamID: "stream", Type: "message", Payload: make([]byte, 64<<10),
		})
		if err != nil {
			t.Fatalf("append %d before acknowledgement: %v", i+1, err)
		}
	}
	if err := j.Acknowledge(ctx, "stream", last.Sequence); err != nil {
		t.Fatalf("acknowledge reusable pages: %v", err)
	}
	db, err := j.dbLocked()
	if err != nil {
		t.Fatal(err)
	}
	freePages := reusableJournalPageBytes(db)
	const writeBytes int64 = 4096
	if freePages <= writeBytes {
		t.Fatalf("free bbolt pages = %d, want more than one write credit %d", freePages, writeBytes)
	}
	j.diskCapacityMu.Lock()
	j.diskCapacityAt = time.Now()
	j.diskCapacityKnown = true
	j.diskAvailable = j.config.DiskReserveBytes + j.config.DiskWriteHeadroomBytes + writeBytes - 1
	j.diskPendingBytes = 0
	j.diskWrittenBytes = 0
	j.diskReusablePendingBytes = freePages - writeBytes
	j.diskCapacityMu.Unlock()

	start := make(chan struct{})
	type reservationResult struct {
		reservation diskCapacityReservation
		err         error
	}
	results := make(chan reservationResult, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			reservation, reserveErr := j.checkDiskCapacity(ctx, db, writeBytes, true)
			results <- reservationResult{reservation: reservation, err: reserveErr}
		}()
	}
	close(start)
	successes := 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err == nil {
			successes++
			j.finishDiskCapacityReservation(result.reservation, false)
		} else if !errors.Is(result.err, ErrJournalFull) {
			t.Fatalf("disk reservation error = %v, want only one request rejected for pressure", result.err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent write reservations succeeded = %d, want exactly one reusable-page credit", successes)
	}
}
