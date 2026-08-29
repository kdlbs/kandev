package sqlite_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	sqlite3 "github.com/mattn/go-sqlite3"
)

type commentWindowTraceState struct {
	countProfiled chan struct{}
	release       chan struct{}
	releaseOnce   sync.Once
	countOnce     sync.Once
}

var commentWindowTraceStateGlobal struct {
	mu    sync.Mutex
	state *commentWindowTraceState
}

func init() {
	sql.Register("sqlite3_comments_window_test", commentWindowDriver{})
}

type commentWindowDriver struct{}

func (commentWindowDriver) Open(name string) (driver.Conn, error) {
	conn, err := (&sqlite3.SQLiteDriver{}).Open(name)
	if err != nil {
		return nil, err
	}
	commentWindowTraceStateGlobal.mu.Lock()
	state := commentWindowTraceStateGlobal.state
	commentWindowTraceStateGlobal.mu.Unlock()
	return &commentWindowConn{Conn: conn, state: state}, nil
}

type commentWindowConn struct {
	driver.Conn
	state *commentWindowTraceState
}

func (c *commentWindowConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.Conn.Prepare(query)
	if err != nil || c.state == nil || !isCommentCountQuery(query) {
		return stmt, err
	}
	return &commentWindowStmt{Stmt: stmt, state: c.state}, nil
}

func (c *commentWindowConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	preparer, ok := c.Conn.(driver.ConnPrepareContext)
	if !ok {
		return c.Prepare(query)
	}
	stmt, err := preparer.PrepareContext(ctx, query)
	if err != nil || c.state == nil || !isCommentCountQuery(query) {
		return stmt, err
	}
	return &commentWindowStmt{Stmt: stmt, state: c.state}, nil
}

func (c *commentWindowConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	beginner, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		legacy, ok := c.Conn.(interface {
			Begin() (driver.Tx, error)
		})
		if !ok {
			return nil, driver.ErrSkip
		}
		return legacy.Begin()
	}
	return beginner.BeginTx(ctx, opts)
}

func (c *commentWindowConn) QueryContext(
	ctx context.Context, query string, args []driver.NamedValue,
) (driver.Rows, error) {
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	rows, err := queryer.QueryContext(ctx, query, args)
	if err != nil || c.state == nil || !isCommentCountQuery(query) {
		return rows, err
	}
	return &commentWindowRows{Rows: rows, state: c.state}, nil
}

func (c *commentWindowConn) ExecContext(
	ctx context.Context, query string, args []driver.NamedValue,
) (driver.Result, error) {
	execer, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return execer.ExecContext(ctx, query, args)
}

type commentWindowRows struct {
	driver.Rows
	state *commentWindowTraceState
}

func (r *commentWindowRows) Next(dest []driver.Value) error {
	err := r.Rows.Next(dest)
	if err == nil {
		r.state.countOnce.Do(func() {
			r.state.countProfiled <- struct{}{}
			<-r.state.release
		})
	}
	return err
}

type commentWindowStmt struct {
	driver.Stmt
	state *commentWindowTraceState
}

func isCommentCountQuery(query string) bool {
	return strings.Contains(strings.ToUpper(query), "SELECT COUNT(*) FROM TASK_COMMENTS")
}

func (s *commentWindowStmt) Query(args []driver.Value) (driver.Rows, error) {
	legacy, ok := s.Stmt.(interface {
		Query([]driver.Value) (driver.Rows, error)
	})
	if !ok {
		return nil, driver.ErrSkip
	}
	rows, err := legacy.Query(args)
	if err != nil {
		return rows, err
	}
	return &commentWindowRows{Rows: rows, state: s.state}, nil
}

func (s *commentWindowStmt) QueryContext(
	ctx context.Context, args []driver.NamedValue,
) (driver.Rows, error) {
	queryer, ok := s.Stmt.(driver.StmtQueryContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	rows, err := queryer.QueryContext(ctx, args)
	if err != nil {
		return rows, err
	}
	return &commentWindowRows{Rows: rows, state: s.state}, nil
}

func newConcurrentCommentWindowRepo(t *testing.T, state *commentWindowTraceState) *sqlite.Repository {
	t.Helper()
	commentWindowTraceStateGlobal.mu.Lock()
	commentWindowTraceStateGlobal.state = state
	commentWindowTraceStateGlobal.mu.Unlock()
	t.Cleanup(func() {
		state.releaseOnce.Do(func() { close(state.release) })
		commentWindowTraceStateGlobal.mu.Lock()
		commentWindowTraceStateGlobal.state = nil
		commentWindowTraceStateGlobal.mu.Unlock()
	})

	dsn := filepath.Join(t.TempDir(), "comments.db") + "?_journal_mode=WAL&_busy_timeout=5000"
	writer, err := sqlx.Open("sqlite3_comments_window_test", dsn)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	reader, err := sqlx.Open("sqlite3_comments_window_test", dsn)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	writer.SetMaxOpenConns(4)
	reader.SetMaxOpenConns(4)
	t.Cleanup(func() {
		_ = writer.Close()
		_ = reader.Close()
	})
	if _, _, err := settingsstore.Provide(writer, reader, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	repo, err := sqlite.NewWithDB(writer, reader, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

// seedComment creates a comment with an explicit CreatedAt so ordering
// assertions don't depend on wall-clock timing between inserts.
func seedComment(t *testing.T, repo interface {
	CreateTaskComment(ctx context.Context, c *models.TaskComment) error
}, taskID, body string, createdAt time.Time) *models.TaskComment {
	t.Helper()
	c := &models.TaskComment{
		TaskID:     taskID,
		AuthorType: "agent",
		AuthorID:   "agent-1",
		Body:       body,
		Source:     "run",
		CreatedAt:  createdAt,
	}
	if err := repo.CreateTaskComment(context.Background(), c); err != nil {
		t.Fatalf("seed comment %q: %v", body, err)
	}
	return c
}

// AC-003.1/AC-003.2/AC-003.3: the window selects the newest `limit`
// comments by (created_at DESC, id DESC) but presents them ascending by
// the same tiebreak columns.
func TestListTaskCommentsWindow_OrdersAscendingAndLimits(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	c1 := seedComment(t, repo, "task-1", "first", base)
	c2 := seedComment(t, repo, "task-1", "second", base.Add(time.Minute))
	c3 := seedComment(t, repo, "task-1", "third", base.Add(2*time.Minute))

	comments, total, err := repo.ListTaskCommentsWindow(ctx, "task-1", 2)
	if err != nil {
		t.Fatalf("ListTaskCommentsWindow: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2", len(comments))
	}
	// Newest 2 by created_at DESC are c3, c2; presented ascending: c2, c3.
	if comments[0].ID != c2.ID || comments[1].ID != c3.ID {
		t.Fatalf("order = [%s, %s], want [%s, %s]", comments[0].ID, comments[1].ID, c2.ID, c3.ID)
	}
	_ = c1
}

// AC-003.3: comments sharing an identical created_at break ties on id
// (DESC for the newest-N selection, which reverses to ASC when presented),
// so ordering is deterministic instead of depending on row/scan order.
func TestListTaskCommentsWindow_TiesBreakOnID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	same := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	c1 := seedComment(t, repo, "task-3", "a", same)
	c2 := seedComment(t, repo, "task-3", "b", same)
	c3 := seedComment(t, repo, "task-3", "c", same)

	byID := []*models.TaskComment{c1, c2, c3}
	sort.Slice(byID, func(i, j int) bool { return byID[i].ID < byID[j].ID })
	wantAsc := []string{byID[0].ID, byID[1].ID, byID[2].ID}

	comments, total, err := repo.ListTaskCommentsWindow(ctx, "task-3", 20)
	if err != nil {
		t.Fatalf("ListTaskCommentsWindow: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(comments) != 3 {
		t.Fatalf("len(comments) = %d, want 3", len(comments))
	}
	gotAsc := []string{comments[0].ID, comments[1].ID, comments[2].ID}
	for i := range wantAsc {
		if gotAsc[i] != wantAsc[i] {
			t.Fatalf("order = %v, want %v (id-ascending tiebreak on equal created_at)", gotAsc, wantAsc)
		}
	}

	// A limit smaller than the tied set must still select the newest-by-id
	// rows (DESC tiebreak for selection), presented ascending.
	limited, _, err := repo.ListTaskCommentsWindow(ctx, "task-3", 2)
	if err != nil {
		t.Fatalf("ListTaskCommentsWindow(limit=2): %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("len(limited) = %d, want 2", len(limited))
	}
	wantLimited := []string{byID[1].ID, byID[2].ID}
	gotLimited := []string{limited[0].ID, limited[1].ID}
	if gotLimited[0] != wantLimited[0] || gotLimited[1] != wantLimited[1] {
		t.Fatalf("limited order = %v, want %v (selects highest-id pair, presented ascending)", gotLimited, wantLimited)
	}
}

// AC-005.1: an empty result set returns a non-nil empty slice, not nil,
// so JSON marshals `[]` rather than `null`.
func TestListTaskCommentsWindow_EmptyReturnsNonNilSlice(t *testing.T) {
	repo := newTestRepo(t)
	comments, total, err := repo.ListTaskCommentsWindow(context.Background(), "no-such-task", 20)
	if err != nil {
		t.Fatalf("ListTaskCommentsWindow: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
	if comments == nil {
		t.Fatal("comments slice must be non-nil (empty), got nil")
	}
	if len(comments) != 0 {
		t.Fatalf("len(comments) = %d, want 0", len(comments))
	}
}

// AC-003.6: total reflects the full comment count on the task, independent
// of the requested limit.
func TestListTaskCommentsWindow_TotalIndependentOfLimit(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		seedComment(t, repo, "task-2", "c", base.Add(time.Duration(i)*time.Minute))
	}
	comments, total, err := repo.ListTaskCommentsWindow(ctx, "task-2", 1)
	if err != nil {
		t.Fatalf("ListTaskCommentsWindow: %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(comments) != 1 {
		t.Fatalf("len(comments) = %d, want 1", len(comments))
	}
}

// AC-003.7/AC-006.1/AC-006.3: the count and page share one read snapshot,
// while a concurrent writer can commit before the page query runs. The trace
// callback gives the writer a deterministic point between those two queries.
func TestListTaskCommentsWindow_SnapshotAllowsConcurrentWrite(t *testing.T) {
	state := &commentWindowTraceState{
		countProfiled: make(chan struct{}, 1),
		release:       make(chan struct{}),
	}
	repo := newConcurrentCommentWindowRepo(t, state)
	ctx := context.Background()
	seedComment(t, repo, "task-concurrent", "before", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	type windowResult struct {
		comments []*models.TaskComment
		total    int
		err      error
	}
	readDone := make(chan windowResult, 1)
	go func() {
		comments, total, err := repo.ListTaskCommentsWindow(ctx, "task-concurrent", 20)
		readDone <- windowResult{comments: comments, total: total, err: err}
	}()

	select {
	case <-state.countProfiled:
	case <-time.After(2 * time.Second):
		t.Fatal("read did not reach the count query")
	}

	writeDone := make(chan error, 1)
	go func() {
		writeDone <- repo.CreateTaskComment(ctx, &models.TaskComment{
			TaskID:     "task-concurrent",
			AuthorType: "agent",
			AuthorID:   "agent-1",
			Body:       "after",
			Source:     "run",
			CreatedAt:  time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC),
		})
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("concurrent write: %v", err)
		}
	case <-time.After(2 * time.Second):
		state.releaseOnce.Do(func() { close(state.release) })
		t.Fatal("concurrent write blocked while the read transaction was paused")
	}
	state.releaseOnce.Do(func() { close(state.release) })

	var got windowResult
	select {
	case got = <-readDone:
	case <-time.After(2 * time.Second):
		t.Fatal("read did not finish after releasing the snapshot barrier")
	}
	if got.err != nil {
		t.Fatalf("ListTaskCommentsWindow: %v", got.err)
	}
	if got.total != 1 || len(got.comments) != 1 || got.comments[0].Body != "before" {
		t.Fatalf("snapshot = total %d comments %v, want the complete pre-write snapshot", got.total, got.comments)
	}

	comments, total, err := repo.ListTaskCommentsWindow(ctx, "task-concurrent", 20)
	if err != nil {
		t.Fatalf("post-write ListTaskCommentsWindow: %v", err)
	}
	if total != 2 || len(comments) != 2 {
		t.Fatalf("post-write window = total %d comments %d, want 2", total, len(comments))
	}
}
