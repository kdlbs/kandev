package messagequeue

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type requeueRepositoryFactory struct {
	name string
	new  func(*testing.T) Repository
}

func requeueRepositoryFactories() []requeueRepositoryFactory {
	return []requeueRepositoryFactory{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
		{name: "postgres", new: newTestPostgresRepo},
	}
}

func insertRequeueTestEntry(t *testing.T, repo Repository, sessionID, content, queuedBy string, metadata map[string]interface{}) *QueuedMessage {
	t.Helper()
	msg := &QueuedMessage{
		SessionID: sessionID,
		TaskID:    "task",
		Content:   content,
		QueuedBy:  queuedBy,
		Metadata:  metadata,
	}
	require.NoError(t, repo.Insert(context.Background(), msg, 0))
	return msg
}

func TestRepository_RequeuePreservingFIFO_NormalizesHeadPosition(t *testing.T) {
	for _, factory := range requeueRepositoryFactories() {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			ctx := context.Background()

			insertRequeueTestEntry(t, repo, "source", "older", QueuedByUser, nil)
			retry := insertRequeueTestEntry(t, repo, "source", "retry", QueuedByUser, nil)
			taken, err := repo.TakeByID(ctx, "source", retry.ID)
			require.NoError(t, err)
			require.Equal(t, retry.ID, taken.ID)

			insertRequeueTestEntry(t, repo, "destination", "destination", QueuedByUser, nil)
			require.NoError(t, repo.RequeuePreservingFIFO(ctx, taken))
			require.NoError(t, repo.TransferSession(ctx, "source", "destination"))

			entries, err := repo.ListBySession(ctx, "destination")
			require.NoError(t, err)
			require.Len(t, entries, 3)
			require.Equal(t, []string{"destination", "retry", "older"}, []string{
				entries[0].Content, entries[1].Content, entries[2].Content,
			})
			require.Less(t, entries[0].Position, entries[1].Position)
			require.Less(t, entries[1].Position, entries[2].Position)
			require.Greater(t, entries[0].Position, int64(0))
		})
	}
}

func TestRepository_RequeuePreservingFIFO_ReplacesPendingCoalesceTarget(t *testing.T) {
	for _, factory := range requeueRepositoryFactories() {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			ctx := context.Background()
			metadata := map[string]interface{}{MetadataCoalesceKey: "ci-key"}

			original := insertRequeueTestEntry(t, repo, "session", "original", QueuedByWorkflow, metadata)
			taken, err := repo.TakeHead(ctx, "session")
			require.NoError(t, err)
			require.Equal(t, original.ID, taken.ID)

			insertRequeueTestEntry(t, repo, "session", "noise", QueuedByUser, nil)
			pending := insertRequeueTestEntry(t, repo, "session", "pending", QueuedByWorkflow, metadata)
			pendingPosition := pending.Position

			require.NoError(t, repo.RequeuePreservingFIFO(ctx, taken))
			entries, err := repo.ListBySession(ctx, "session")
			require.NoError(t, err)
			require.Len(t, entries, 2)
			require.Equal(t, "noise", entries[0].Content)
			require.Equal(t, "original", entries[1].Content)
			require.Equal(t, pending.ID, entries[1].ID)
			require.Equal(t, pendingPosition, entries[1].Position)
		})
	}
}

func TestRepository_RequeuePreservingFIFO_EmptyQueueStartsAtOne(t *testing.T) {
	for _, factory := range requeueRepositoryFactories() {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			ctx := context.Background()
			msg := &QueuedMessage{SessionID: "session", TaskID: "task", Content: "retry", QueuedBy: QueuedByUser}

			require.NoError(t, repo.RequeuePreservingFIFO(ctx, msg))
			require.Equal(t, int64(1), msg.Position)
			entries, err := repo.ListBySession(ctx, "session")
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.Equal(t, int64(1), entries[0].Position)
		})
	}
}

type requeueTrackingRepository struct {
	Repository
	called chan struct{}
}

func (r *requeueTrackingRepository) RequeuePreservingFIFO(ctx context.Context, msg *QueuedMessage) error {
	close(r.called)
	return r.Repository.RequeuePreservingFIFO(ctx, msg)
}

func TestService_RequeueAtHeadWaitsForSessionAdmission(t *testing.T) {
	repo := &requeueTrackingRepository{
		Repository: NewMemoryRepository(),
		called:     make(chan struct{}),
	}
	svc := newAutoMergeTestServiceWithRepository(t, repo, 10)
	svc.SetAutoMergeEnabled(false)
	ctx := context.Background()

	queued, err := svc.QueueMessage(ctx, "session", "task", "retry", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	taken, ok, err := svc.TakeQueuedEntry(ctx, "session", queued.ID)
	require.NoError(t, err)
	require.True(t, ok)

	holderStarted := make(chan struct{})
	releaseHolder := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- svc.WithSessionAdmission(ctx, "session", func(context.Context) error {
			close(holderStarted)
			<-releaseHolder
			return nil
		})
	}()
	<-holderStarted

	requeueDone := make(chan error, 1)
	go func() { requeueDone <- svc.RequeueAtHead(ctx, taken) }()
	select {
	case <-repo.called:
		t.Fatal("requeue reached the repository while session admission was held")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseHolder)
	require.NoError(t, <-holderDone)
	require.NoError(t, <-requeueDone)
	select {
	case <-repo.called:
	case <-time.After(time.Second):
		t.Fatal("requeue did not reach the repository after admission was released")
	}
}
