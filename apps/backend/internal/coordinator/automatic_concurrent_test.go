package coordinator

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	internaldb "github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// newAutomaticServiceOn builds a raised phase 3 service on store, which may
// be SQLite or PostgreSQL.
func newAutomaticServiceOn(t *testing.T, store *Store) (*Coordinator, *Service) {
	t.Helper()
	store.now = func() time.Time { return automaticNow }
	c := newTestCoordinator(t, store, "ws-1")
	tasks := &fakeDecisionTaskService{
		workflows:    map[string]*taskmodels.Workflow{"wf-1": {ID: "wf-1", WorkspaceID: "ws-1"}},
		repos:        map[string]*taskmodels.Repository{"repo-1": {ID: "repo-1", WorkspaceID: "ws-1"}},
		tasks:        map[string]*taskmodels.Task{"task-0": {ID: "task-0", WorkspaceID: "ws-1"}},
		createResult: createdResult("task-new"),
		settled:      true,
	}
	steps := &fakeStepReader{steps: []*workflowmodels.WorkflowStep{{ID: "step-1", IsStartStep: true}}}
	az := &identityAuthorizer{allowed: map[string]bool{testRaiser: true}}
	svc := NewService(store, newValidatorForTest(nil, nil), az, newTestLogger(t))
	svc.SetDecisionDeps(tasks, steps, nil)
	svc.phase2, svc.phase3 = true, true
	svc.SetAutomaticIdentities(fakeIdentities{known: map[string]bool{testRaiser: true}})
	earliest := automaticNow.Add(-31 * 24 * time.Hour)
	svc.SetAutomaticPorts(nil, &fakeDecisionLog{earliest: &earliest, rows: decisions(20, 20)})
	recordReview(t, store, c, automaticNow.Add(-time.Hour))
	if _, err := svc.SaveSettings(authedContext(testRaiser), c.WorkspaceID, c.ID, []byte(raiseBody())); err != nil {
		t.Fatalf("raise: %v", err)
	}
	return c, svc
}

func seedAutomaticApprovals(t *testing.T, store *Store, c *Coordinator, n int) {
	t.Helper()
	at := automaticNow.Add(-time.Hour)
	for i := 0; i < n; i++ {
		mustExec(t, store, `INSERT INTO coordinator_proposals
			(id, coordinator_id, workspace_id, status, spec_json, created_at, updated_at, kind, decided_automatically, claimed_automatically, automatic_at)
			VALUES (?, ?, ?, 'approved', '{}', ?, ?, 'create_task', 1, 1, ?)`,
			fmt.Sprintf("seed-%d", i), c.ID, c.WorkspaceID, at, at, at)
	}
}

func runConcurrentTenth(t *testing.T, store *Store) {
	t.Helper()
	c, svc := newAutomaticServiceOn(t, store)
	seedAutomaticApprovals(t, store, c, automaticDailyLimit-1)
	first, second := insertKind(t, store, c, ProposalKindCreateTask), insertKind(t, store, c, ProposalKindCreateTask)
	start := make(chan struct{})
	results := make([]*AutomaticResult, 2)
	var wg sync.WaitGroup
	for i, p := range []*Proposal{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := svc.TryAutomaticApproval(context.Background(), p)
			if err != nil {
				t.Error(err)
			}
			results[i] = res
		}()
	}
	close(start)
	wg.Wait()
	approved, limited := 0, 0
	for _, res := range results {
		switch {
		case res != nil && res.Status == ProposalStatusApproved:
			approved++
		case res != nil && res.Note == limitReachedNote:
			limited++
		}
	}
	if approved != 1 || limited != 1 {
		t.Fatalf("approved=%d limited=%d results=%+v %+v", approved, limited, results[0], results[1])
	}
	n, err := store.CountAutomaticDecidedTx(context.Background(), store.db, c.ID, automaticNow.Add(-automaticLimitWindow))
	if err != nil || n != automaticDailyLimit {
		t.Fatalf("count = %d err = %v", n, err)
	}
}

func TestAutomaticApproval_ConcurrentTenthSQLite(t *testing.T) {
	runConcurrentTenth(t, newTestStore(t))
}

// newPooledPostgresStore opens a multi-connection pool whose search_path is a
// connection parameter, so a transaction and a read on the same pool do not
// wait on each other the way they do on the single-connection test helper.
func newPooledPostgresStore(t *testing.T) *Store {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := internaldb.OpenPostgres(dsn, 1, 1)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
		_ = admin.Close()
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	raw, err := internaldb.OpenPostgres(u.String(), 8, 2)
	if err != nil {
		t.Fatalf("open pooled postgres: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	pg := sqlx.NewDb(raw, "pgx")
	store, err := NewStore(pg, pg)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestAutomaticApproval_ConcurrentTenthPostgres(t *testing.T) {
	runConcurrentTenth(t, newPooledPostgresStore(t))
}
