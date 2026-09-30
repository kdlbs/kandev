package coordinator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
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

func TestAutomaticApproval_ConcurrentTenthPostgres(t *testing.T) {
	runConcurrentTenth(t, newTestStorePostgres(t))
}
