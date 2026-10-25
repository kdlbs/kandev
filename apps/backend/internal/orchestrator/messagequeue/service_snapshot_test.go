package messagequeue

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

type queueSnapshotter interface {
	Snapshot(context.Context, QueueSessionIdentity) (*QueueStatus, error)
}

type pausingSnapshotRepository struct {
	Repository
	firstCaptured chan struct{}
	releaseFirst  chan struct{}
	calls         atomic.Int32
}

func (r *pausingSnapshotRepository) Snapshot(
	ctx context.Context,
	identity QueueSessionIdentity,
) (RepositorySnapshot, error) {
	snapshot, err := r.Repository.Snapshot(ctx, identity)
	if r.calls.Add(1) == 1 {
		close(r.firstCaptured)
		<-r.releaseFirst
	}
	return snapshot, err
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.5
// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.15
func TestSnapshotProjectsIdentityAndEffectiveAutoMergePolicy(t *testing.T) {
	repository := NewMemoryRepository()
	service := NewService(repository, 7, logger.Default())
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation"}
	seedQueueSessionIdentity(t, repository, identity)
	service.SetAutoMergePolicy(true, 8)
	if _, err := service.SetSessionAutoMerge(context.Background(), identity, false); err != nil {
		t.Fatalf("set session Auto-merge: %v", err)
	}
	snapshots, ok := interface{}(service).(queueSnapshotter)
	if !ok {
		t.Fatal("message queue service does not expose identity-bound snapshots")
	}

	status, err := snapshots.Snapshot(context.Background(), identity)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if status.TaskID != identity.TaskID || status.SessionID != identity.SessionID || status.SessionIncarnationID != identity.SessionIncarnationID {
		t.Fatalf("snapshot identity = %+v", status)
	}
	if !status.AutoMergeAvailable || status.AutoMergeEnabled == nil || *status.AutoMergeEnabled ||
		status.AutoMergeSource != AutoMergeSourceSession ||
		status.AutoMergeRevision == nil || *status.AutoMergeRevision != 1 {
		t.Fatalf("snapshot Auto-merge policy = %+v", status)
	}
	if status.StatusEpoch == "" || status.StatusEpoch == identity.SessionIncarnationID || status.Max != 7 {
		t.Fatalf("snapshot ordering/capacity = %+v", status)
	}
	second, err := snapshots.Snapshot(context.Background(), identity)
	if err != nil {
		t.Fatalf("second snapshot: %v", err)
	}
	if second.StatusEpoch != status.StatusEpoch || second.StatusGeneration <= status.StatusGeneration {
		t.Fatalf("snapshot ordering did not advance in epoch: first=%+v second=%+v", status, second)
	}
}

func TestSnapshotGenerationIsSharedAcrossBackendServices(t *testing.T) {
	repository := newTestSQLiteRepo(t)
	firstService := NewService(repository, 7, logger.Default())
	secondService := NewService(repository, 7, logger.Default())
	identity := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation",
	}
	seedQueueSessionIdentity(t, repository, identity)

	first, err := firstService.Snapshot(context.Background(), identity)
	if err != nil {
		t.Fatalf("first backend snapshot: %v", err)
	}
	second, err := secondService.Snapshot(context.Background(), identity)
	if err != nil {
		t.Fatalf("second backend snapshot: %v", err)
	}
	if first.StatusEpoch == second.StatusEpoch {
		t.Fatalf("backend epochs unexpectedly match: %q", first.StatusEpoch)
	}
	if second.StatusGeneration <= first.StatusGeneration {
		t.Fatalf("shared generation did not advance: first=%+v second=%+v", first, second)
	}
}

func TestSnapshotGenerationOrdersConcurrentSnapshotStarts(t *testing.T) {
	baseRepository := NewMemoryRepository()
	repository := &pausingSnapshotRepository{
		Repository:    baseRepository,
		firstCaptured: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
	service := NewService(repository, 7, logger.Default())
	identity := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation",
	}
	seedQueueSessionIdentity(t, baseRepository, identity)

	firstResult := make(chan *QueueStatus, 1)
	firstErr := make(chan error, 1)
	go func() {
		status, err := service.Snapshot(context.Background(), identity)
		firstResult <- status
		firstErr <- err
	}()
	<-repository.firstCaptured
	if err := baseRepository.Insert(context.Background(), &QueuedMessage{
		ID: "new-entry", TaskID: identity.TaskID, SessionID: identity.SessionID, Content: "new",
	}, 7); err != nil {
		t.Fatalf("insert between snapshots: %v", err)
	}
	second, err := service.Snapshot(context.Background(), identity)
	if err != nil {
		t.Fatalf("second snapshot: %v", err)
	}
	close(repository.releaseFirst)
	first := <-firstResult
	if err := <-firstErr; err != nil {
		t.Fatalf("first snapshot: %v", err)
	}

	if first.Count != 0 || second.Count != 1 {
		t.Fatalf("snapshot setup failed: first=%+v second=%+v", first, second)
	}
	if first.StatusGeneration >= second.StatusGeneration {
		t.Fatalf("snapshot generations do not preserve start order: first=%d second=%d",
			first.StatusGeneration, second.StatusGeneration)
	}
}

func TestSnapshotPreservesEntriesAndReportsUnavailablePolicy(t *testing.T) {
	baseRepository := NewMemoryRepository()
	repository := &failingAutoMergeReadRepository{Repository: baseRepository}
	service := NewService(repository, 7, logger.Default())
	identity := QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "incarnation",
	}
	seedQueueSessionIdentity(t, baseRepository, identity)
	if err := repository.Insert(context.Background(), &QueuedMessage{
		ID: "entry", TaskID: identity.TaskID, SessionID: identity.SessionID, Content: "queued",
	}, 7); err != nil {
		t.Fatalf("insert queue entry: %v", err)
	}

	status, err := service.Snapshot(context.Background(), identity)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if status.AutoMergeAvailable {
		t.Fatalf("AutoMergeAvailable = true, want false: %+v", status)
	}
	if status.AutoMergeEnabled != nil || status.AutoMergeSource != "" || status.AutoMergeRevision != nil {
		t.Fatalf("unavailable policy leaked values: %+v", status)
	}
	if status.Count != 1 || len(status.Entries) != 1 || status.Entries[0].ID != "entry" {
		t.Fatalf("snapshot entries = %+v, want preserved entry", status)
	}
}
