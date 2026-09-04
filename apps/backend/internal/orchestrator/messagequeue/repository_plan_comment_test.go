package messagequeue_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

type planCommentQueueWriter interface {
	InsertWithPlanComments(
		context.Context,
		*messagequeue.QueuedMessage,
		[]models.TaskPlanCommentRef,
		bool,
		int,
	) (*models.TaskPlanCommentSnapshot, bool, error)
}

func TestSQLiteRepositoryInsertWithPlanCommentsConsumesAtomically(t *testing.T) {
	taskRepo, queueRepo := newPlanCommentQueueRepos(t)
	ctx := context.Background()
	seedQueuePlanComment(t, ctx, taskRepo, "atomic")
	writer := requirePlanCommentQueueWriter(t, queueRepo)
	refs := []models.TaskPlanCommentRef{{ID: "comment-atomic", Version: 1}}
	queued := planCommentQueuedMessage("atomic", "queue-comment-atomic", "fingerprint-atomic", refs)

	snapshot, replay, err := writer.InsertWithPlanComments(ctx, queued, refs, true, 10)
	if err != nil {
		t.Fatalf("InsertWithPlanComments: %v", err)
	}
	if replay {
		t.Fatal("first insert reported replay")
	}
	if snapshot.Revision != 2 || len(snapshot.Comments) != 0 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	want := "### Plan Comments\n\n```\nselected\n```\n> stored atomic\n\n---\n\ntyped content"
	if queued.Content != want {
		t.Fatalf("queued content = %q, want %q", queued.Content, want)
	}
	entries, err := queueRepo.ListBySession(ctx, queued.SessionID)
	if err != nil || len(entries) != 1 || entries[0].Content != want {
		t.Fatalf("stored queue = %#v, err=%v", entries, err)
	}
}

func TestSQLiteRepositoryInsertWithPlanCommentsReplayIsIdempotent(t *testing.T) {
	taskRepo, queueRepo := newPlanCommentQueueRepos(t)
	ctx := context.Background()
	seedQueuePlanComment(t, ctx, taskRepo, "replay")
	writer := requirePlanCommentQueueWriter(t, queueRepo)
	refs := []models.TaskPlanCommentRef{{ID: "comment-replay", Version: 1}}
	first := planCommentQueuedMessage("replay", "queue-comment-replay", "fingerprint-replay", refs)
	if _, replay, err := writer.InsertWithPlanComments(ctx, first, refs, true, 10); err != nil || replay {
		t.Fatalf("first insert replay=%v err=%v", replay, err)
	}

	retry := planCommentQueuedMessage("replay", first.ID, "fingerprint-replay", refs)
	snapshot, replay, err := writer.InsertWithPlanComments(ctx, retry, refs, true, 10)
	if err != nil || !replay || snapshot != nil {
		t.Fatalf("retry snapshot=%#v replay=%v err=%v", snapshot, replay, err)
	}
	if retry.Content != first.Content || retry.Position != first.Position {
		t.Fatalf("replayed row = %#v, want %#v", retry, first)
	}
	entries, err := queueRepo.ListBySession(ctx, first.SessionID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %#v, err=%v", entries, err)
	}

	conflict := planCommentQueuedMessage("replay", first.ID, "different-fingerprint", refs)
	if _, _, err := writer.InsertWithPlanComments(ctx, conflict, refs, true, 10); !errors.Is(err, messagequeue.ErrQueueIDConflict) {
		t.Fatalf("conflicting replay error = %v, want ErrQueueIDConflict", err)
	}
	pending, err := taskRepo.ListTaskPlanComments(ctx, first.TaskID)
	if err != nil || pending.Revision != 2 || len(pending.Comments) != 0 {
		t.Fatalf("pending snapshot = %#v, err=%v", pending, err)
	}
}

func TestSQLiteRepositoryInsertWithPlanCommentsFailuresPreserveComments(t *testing.T) {
	tests := []struct {
		name       string
		prepare    func(t *testing.T, ctx context.Context, taskRepo *tasksqlite.Repository, queueRepo messagequeue.Repository, queued *messagequeue.QueuedMessage)
		refs       []models.TaskPlanCommentRef
		requirePri bool
		max        int
		assertErr  func(error) bool
	}{
		{
			name: "stale reference",
			refs: []models.TaskPlanCommentRef{{ID: "comment-failure", Version: 9}},
			max:  10,
			assertErr: func(err error) bool {
				var changed *plancommenttx.CommentsChangedError
				return errors.As(err, &changed) && changed.Snapshot != nil
			},
		},
		{
			name: "queue full",
			prepare: func(t *testing.T, ctx context.Context, _ *tasksqlite.Repository, queueRepo messagequeue.Repository, queued *messagequeue.QueuedMessage) {
				t.Helper()
				if err := queueRepo.Insert(ctx, &messagequeue.QueuedMessage{
					ID: "existing", SessionID: queued.SessionID, TaskID: queued.TaskID, QueuedBy: messagequeue.QueuedByUser,
				}, 10); err != nil {
					t.Fatal(err)
				}
			},
			refs:      []models.TaskPlanCommentRef{{ID: "comment-failure", Version: 1}},
			max:       1,
			assertErr: func(err error) bool { return errors.Is(err, messagequeue.ErrQueueFull) },
		},
		{
			name: "stale primary",
			prepare: func(t *testing.T, ctx context.Context, taskRepo *tasksqlite.Repository, _ messagequeue.Repository, queued *messagequeue.QueuedMessage) {
				t.Helper()
				secondaryID := "session-failure-secondary"
				if err := taskRepo.CreateTaskSession(ctx, &models.TaskSession{
					ID: secondaryID, TaskID: queued.TaskID, State: models.TaskSessionStateWaitingForInput,
				}); err != nil {
					t.Fatal(err)
				}
				queued.SessionID = secondaryID
			},
			refs:       []models.TaskPlanCommentRef{{ID: "comment-failure", Version: 1}},
			requirePri: true,
			max:        10,
			assertErr: func(err error) bool {
				var changed *plancommenttx.PrimarySessionChangedError
				return errors.As(err, &changed) && changed.SessionID == "session-failure"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			taskRepo, queueRepo := newPlanCommentQueueRepos(t)
			ctx := context.Background()
			seedQueuePlanComment(t, ctx, taskRepo, "failure")
			queued := planCommentQueuedMessage("failure", "queue-comment-failure", "fingerprint-failure", test.refs)
			if test.prepare != nil {
				test.prepare(t, ctx, taskRepo, queueRepo, queued)
			}

			snapshot, replay, err := requirePlanCommentQueueWriter(t, queueRepo).
				InsertWithPlanComments(ctx, queued, test.refs, test.requirePri, test.max)
			if !test.assertErr(err) || snapshot != nil || replay {
				t.Fatalf("result snapshot=%#v replay=%v err=%#v", snapshot, replay, err)
			}
			entries, listErr := queueRepo.ListBySession(ctx, queued.SessionID)
			if listErr != nil || (test.name != "queue full" && len(entries) != 0) {
				t.Fatalf("queue after failure = %#v, err=%v", entries, listErr)
			}
			pending, listErr := taskRepo.ListTaskPlanComments(ctx, queued.TaskID)
			if listErr != nil || pending.Revision != 1 || len(pending.Comments) != 1 {
				t.Fatalf("pending snapshot = %#v, err=%v", pending, listErr)
			}
		})
	}
}

func TestPostgresInsertWithPlanCommentsConcurrentQueueIDConflictIsTyped(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	taskRepo, err := tasksqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	queueRepoA, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatal(err)
	}
	queueRepoB, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seedQueuePlanComment(t, ctx, taskRepo, "race-a")
	seedQueuePlanComment(t, ctx, taskRepo, "race-b")
	refsA := []models.TaskPlanCommentRef{{ID: "comment-race-a", Version: 1}}
	refsB := []models.TaskPlanCommentRef{{ID: "comment-race-b", Version: 1}}
	messages := []*messagequeue.QueuedMessage{
		planCommentQueuedMessage("race-a", "shared-client-queue-id", "fingerprint-a", refsA),
		planCommentQueuedMessage("race-b", "shared-client-queue-id", "fingerprint-b", refsB),
	}
	writers := []planCommentQueueWriter{
		requirePlanCommentQueueWriter(t, queueRepoA), requirePlanCommentQueueWriter(t, queueRepoB),
	}
	refs := [][]models.TaskPlanCommentRef{refsA, refsB}
	start := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, _, errs[index] = writers[index].InsertWithPlanComments(ctx, messages[index], refs[index], true, 10)
		}(i)
	}
	close(start)
	wg.Wait()

	succeeded, conflicted := 0, 0
	for _, insertErr := range errs {
		switch {
		case insertErr == nil:
			succeeded++
		case errors.Is(insertErr, messagequeue.ErrQueueIDConflict):
			conflicted++
		default:
			t.Fatalf("concurrent insert returned untyped error: %v", insertErr)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent results = %#v, want one success and one conflict", errs)
	}
	pendingA, err := taskRepo.ListTaskPlanComments(ctx, "task-race-a")
	if err != nil {
		t.Fatal(err)
	}
	pendingB, err := taskRepo.ListTaskPlanComments(ctx, "task-race-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(pendingA.Comments)+len(pendingB.Comments) != 1 {
		t.Fatalf("losing comment was not preserved: A=%#v B=%#v", pendingA, pendingB)
	}
}

func newPlanCommentQueueRepos(t *testing.T) (*tasksqlite.Repository, messagequeue.Repository) {
	t.Helper()
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "plan-comment-queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	taskRepo, err := tasksqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatal(err)
	}
	return taskRepo, queueRepo
}

func requirePlanCommentQueueWriter(t *testing.T, repo messagequeue.Repository) planCommentQueueWriter {
	t.Helper()
	writer, ok := repo.(planCommentQueueWriter)
	if !ok {
		t.Fatal("queue repository does not implement atomic plan-comment admission")
	}
	return writer
}

func seedQueuePlanComment(t *testing.T, ctx context.Context, repo *tasksqlite.Repository, suffix string) {
	t.Helper()
	taskID := "task-" + suffix
	sessionID := "session-" + suffix
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetSessionPrimary(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{ID: "plan-" + suffix, TaskID: taskID, Content: "Plan"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-" + suffix, TaskID: taskID, PlanID: "plan-" + suffix,
		Body: "stored " + suffix, SelectedText: "selected", AnchorFrom: 1, AnchorTo: 4,
	}); err != nil {
		t.Fatal(err)
	}
}

func planCommentQueuedMessage(
	suffix, id, fingerprint string,
	refs []models.TaskPlanCommentRef,
) *messagequeue.QueuedMessage {
	return &messagequeue.QueuedMessage{
		ID: id, SessionID: "session-" + suffix, TaskID: "task-" + suffix,
		Content: plancomments.WithPlaceholder("typed content"), QueuedBy: messagequeue.QueuedByUser,
		Metadata: map[string]interface{}{
			plancomments.MetadataRefs: refs, plancomments.MetadataRequestFingerprint: fingerprint,
		},
	}
}
