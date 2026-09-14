package messagequeue

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type blockingPurgeRepository struct {
	Repository
	purgeStarted  chan struct{}
	purgeRelease  chan struct{}
	listStarted   chan struct{}
	insertStarted chan struct{}
	insertRelease chan struct{}
	listOnce      sync.Once
}

func (r *blockingPurgeRepository) PurgeTask(ctx context.Context, taskID string) (int, error) {
	close(r.purgeStarted)
	<-r.purgeRelease
	return r.Repository.PurgeTask(ctx, taskID)
}

func (r *blockingPurgeRepository) ListBySession(ctx context.Context, sessionID string) ([]QueuedMessage, error) {
	r.listOnce.Do(func() { close(r.listStarted) })
	return r.Repository.ListBySession(ctx, sessionID)
}

func (r *blockingPurgeRepository) Insert(ctx context.Context, msg *QueuedMessage, maxPerSession int) error {
	r.blockInsert()
	return r.Repository.Insert(ctx, msg, maxPerSession)
}

func (r *blockingPurgeRepository) InsertForSession(ctx context.Context, identity QueueSessionIdentity, msg *QueuedMessage, maxPerSession int) error {
	r.blockInsert()
	return r.Repository.InsertForSession(ctx, identity, msg, maxPerSession)
}

func (r *blockingPurgeRepository) blockInsert() {
	if r.insertStarted != nil {
		close(r.insertStarted)
		<-r.insertRelease
	}
}

func TestPurgeTaskWaitsForInFlightQueueAdmission(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	blocking := &blockingPurgeRepository{
		Repository:    svc.repo,
		purgeStarted:  make(chan struct{}),
		purgeRelease:  make(chan struct{}),
		insertStarted: make(chan struct{}),
		insertRelease: make(chan struct{}),
	}
	svc.repo = blocking

	queueDone := make(chan error, 1)
	go func() {
		_, queueErr := svc.QueueMessage(ctx, "session-purge-admission", "task-purge-admission", "queued", "", QueuedByUser, false, nil)
		queueDone <- queueErr
	}()
	<-blocking.insertStarted

	purgeDone := make(chan error, 1)
	go func() {
		_, purgeErr := svc.PurgeTask(ctx, "task-purge-admission")
		purgeDone <- purgeErr
	}()

	select {
	case <-blocking.purgeStarted:
		t.Fatal("PurgeTask reached the repository while queue admission was in flight")
	case <-time.After(100 * time.Millisecond):
	}

	close(blocking.insertRelease)
	require.NoError(t, <-queueDone)
	select {
	case <-blocking.purgeStarted:
	case <-time.After(time.Second):
		t.Fatal("PurgeTask did not proceed after queue admission completed")
	}
	close(blocking.purgeRelease)
	require.NoError(t, <-purgeDone)
}

func TestPurgeTaskBlocksBeginEditUntilLeaseInvalidation(t *testing.T) {
	svc := setupService(t)
	ctx := context.Background()
	entry, err := svc.QueueMessage(ctx, "session-purge-race", "task-purge-race", "body", "", QueuedByUser, false, nil)
	require.NoError(t, err)

	blocking := &blockingPurgeRepository{
		Repository:   svc.repo,
		purgeStarted: make(chan struct{}),
		purgeRelease: make(chan struct{}),
		listStarted:  make(chan struct{}),
	}
	svc.repo = blocking

	purgeDone := make(chan error, 1)
	go func() {
		_, purgeErr := svc.PurgeTask(ctx, entry.TaskID)
		purgeDone <- purgeErr
	}()
	<-blocking.purgeStarted

	beginDone := make(chan error, 1)
	go func() {
		_, beginErr := svc.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
		beginDone <- beginErr
	}()

	select {
	case <-blocking.listStarted:
		t.Fatal("BeginEdit read the queue while PurgeTask was in progress")
	case <-time.After(100 * time.Millisecond):
	}

	close(blocking.purgeRelease)
	require.NoError(t, <-purgeDone)
	require.ErrorIs(t, <-beginDone, ErrEditLeaseNotFound)
}
