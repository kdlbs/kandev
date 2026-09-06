package orchestrator

// Regression coverage for the operation ledger's lifetime: it must survive
// initWorkflowEngine rebuilding everything else around it. See
// docs/specs/workflow-engine-operation-ledger-lifetime/spec.md.

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// AC-L3a: a ledger at its zero value reports every operation id as not
// applied, with no pre-seeding or hydration.
func TestOperationLedgerZeroValueReportsNothingApplied(t *testing.T) {
	var ledger operationLedger
	if ledger.isApplied("anything") {
		t.Fatal("expected zero-value ledger to report nothing applied")
	}
}

// AC-L6/AC-S6: concurrent access needs no external locking, and two
// goroutines checking the same unmarked id may both observe "not applied"
// and proceed — the ledger is a memo, not a mutex. Run with -race.
func TestOperationLedgerConcurrentAccess(t *testing.T) {
	var ledger operationLedger

	start := make(chan struct{})
	results := make(chan bool, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			<-start
			results <- ledger.isApplied("op-race")
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for applied := range results {
		if applied {
			t.Fatal("expected both concurrent checks before any mark to observe not-applied")
		}
	}

	const workers = 50
	var busy sync.WaitGroup
	busy.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer busy.Done()
			ledger.isApplied("op-shared")
			ledger.markApplied("op-shared")
			ledger.isApplied("op-shared")
		}()
	}
	busy.Wait()
	if !ledger.isApplied("op-shared") {
		t.Fatal("expected op-shared to be marked applied after concurrent access")
	}
}

// AC-T1/AC-L1/AC-L2: an operation marked applied stays applied through the
// *new* store a Set*-triggered reinitWorkflowEngine builds — asserting only
// that the pre-reinit store still remembers it would not exercise the fix,
// since production stops reading that store the moment reinit runs.
func TestOperationLedgerSurvivesReinitWorkflowEngine(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)

	if err := svc.workflowStore.MarkOperationApplied(ctx, "op-1"); err != nil {
		t.Fatalf("MarkOperationApplied: %v", err)
	}

	oldStore := svc.workflowStore
	svc.SetReviewRunner(&fakeReviewRunner{})
	if svc.workflowStore == oldStore {
		t.Fatal("expected SetReviewRunner to rebuild workflowStore via reinitWorkflowEngine")
	}

	applied, err := svc.workflowStore.IsOperationApplied(ctx, "op-1")
	if err != nil {
		t.Fatalf("IsOperationApplied: %v", err)
	}
	if !applied {
		t.Fatal("expected op-1 to still read applied through the store built after reinit")
	}
}

// AC-T2 (engine class): dedup across a reinit for a trigger the engine
// itself checks — on_agent_error through HandleTrigger. A second delivery of
// the identical failure after a Set*-triggered reinit must not re-run the
// step's action.
func TestDispatchKanbanAgentErrorTrigger_DedupedAcrossReinit(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{
			OnAgentError: []wfmodels.GenericAction{{Type: wfmodels.GenericActionClearDecisions}},
		},
	}

	decisions := &spyDecisionStore{}
	svc, _ := newAgentErrorTestService(t, repo, stepGetter, func(s *Service) {
		s.engineDecisions = decisions
	})

	failure := watcher.AgentEventData{TaskID: "t1", SessionID: "s1", AgentExecutionID: "exec-1", ErrorMessage: "boom"}
	svc.handleRecoverableFailureLocked(ctx, failure)
	if decisions.clearCalls != 1 {
		t.Fatalf("clearCalls after first delivery = %d, want 1", decisions.clearCalls)
	}

	// A Set* call anywhere between the two deliveries rebuilds the engine,
	// store and agentErrorDeps — the exact boot-time window this card fixes.
	svc.SetReviewRunner(&fakeReviewRunner{})

	svc.handleRecoverableFailureLocked(ctx, failure)
	if decisions.clearCalls != 1 {
		t.Fatalf("clearCalls after redelivery across reinit = %d, want 1 (idempotent)", decisions.clearCalls)
	}
}

// AC-T2 (orchestrator class): dedup across a reinit for a trigger the
// orchestrator checks directly — on_children_completed through
// processOnChildrenCompleted, with the child-row set unchanged between
// deliveries.
//
// step_mid also declares OnChildrenCompleted so a ledger miss on the second
// delivery would not merely be harmless: childCompletionOperationID is
// derived from the (unchanged) child rows, not the parent's current step, so
// without the ledger the redelivery would re-evaluate from step_mid's own
// config and move the parent a second time, to step_done. A chain where the
// destination has no further OnChildrenCompleted action would pass this test
// even with the pre-fix ledger, since nothing would be left to re-trigger.
func TestProcessOnChildrenCompleted_DedupedAcrossReinit(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_wait"] = &wfmodels.WorkflowStep{
		ID: "step_wait", WorkflowID: "wf1", Name: "Wait for Subtasks", Position: 0,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{{Type: wfmodels.GenericActionMoveToNext}},
		},
	}
	stepGetter.steps["step_mid"] = &wfmodels.WorkflowStep{
		ID: "step_mid", WorkflowID: "wf1", Name: "Mid", Position: 1,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{{Type: wfmodels.GenericActionMoveToNext}},
		},
	}
	stepGetter.steps["step_done"] = &wfmodels.WorkflowStep{
		ID: "step_done", WorkflowID: "wf1", Name: "Done", Position: 2,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	// step_mid is not terminal (step_done follows it), so the first
	// transition's lifecycle writes a REVIEW cache state through
	// svc.taskRepo; seed it so that write does not log a harmless
	// "task not found" error against the mock cache.
	if mockRepo, ok := svc.taskRepo.(*mockTaskRepo); ok {
		seedMockTaskState(mockRepo, "parent", v1.TaskStateInProgress)
	}
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterDone <- struct{}{}:
		default:
		}
	}

	now := time.Now().UTC()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "child-complete", WorkflowID: "wf1", Title: "Complete child",
		State: v1.TaskStateCompleted, ParentID: "parent", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create child: %v", err)
	}

	if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); !transitioned {
		t.Fatal("expected all-terminal active children to transition parent from step_wait to step_mid")
	}
	waitForChildrenCompletedOnEnter(t, onEnterDone)
	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if parent.WorkflowStepID != "step_mid" {
		t.Fatalf("expected parent on step_mid after first delivery, got %q", parent.WorkflowStepID)
	}

	// A Set* call between deliveries rebuilds workflowEngine/workflowStore;
	// the same child rows (same operation id) must still dedup.
	svc.SetReviewRunner(&fakeReviewRunner{})

	if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); transitioned {
		t.Fatal("expected redelivery of the identical child-row set to be deduped across reinit, not re-evaluated from step_mid")
	}
	parent, err = repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent after redelivery: %v", err)
	}
	if parent.WorkflowStepID != "step_mid" {
		t.Fatalf("expected parent to remain on step_mid, got %q (a second transition ran)", parent.WorkflowStepID)
	}
}

// AC-T3(a)/AC-L4: a Service on which initWorkflowEngine never ran (workflowStore
// is nil) still has a usable ledger — observed directly on the field, not
// through a dispatch path, since processOnChildrenCompleted and
// switchWorkflowDispatcher both guard on workflowStore/workflowEngine being
// non-nil and make that route unreachable.
func TestOperationLedgerUsableBeforeWorkflowStepGetterSet(t *testing.T) {
	svc := &Service{}
	if svc.workflowStore != nil {
		t.Fatal("expected workflowStore to be nil before SetWorkflowStepGetter")
	}

	svc.operationLedger.markApplied("op-1")
	if !svc.operationLedger.isApplied("op-1") {
		t.Fatal("expected a Service with no workflowStepGetter wired to still have a usable ledger")
	}
}

// AC-T3(b)/AC-S3a: a bare &Service{} literal that bypasses NewService, wired
// only through SetWorkflowStepGetter, still produces a workflowStore that
// resolves to that same Service's ledger in both directions.
func TestBareServiceLiteralWorkflowStoreSharesItsOwnLedger(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)

	if err := svc.workflowStore.MarkOperationApplied(ctx, "op-a"); err != nil {
		t.Fatalf("MarkOperationApplied: %v", err)
	}
	if !svc.operationLedger.isApplied("op-a") {
		t.Fatal("expected the Service's own ledger field to observe a mark made through its workflowStore")
	}

	svc.operationLedger.markApplied("op-b")
	applied, err := svc.workflowStore.IsOperationApplied(ctx, "op-b")
	if err != nil {
		t.Fatalf("IsOperationApplied: %v", err)
	}
	if !applied {
		t.Fatal("expected workflowStore to observe a mark made directly on the Service's ledger field")
	}
}

// AC-T4/AC-L3/AC-B2: initWorkflowEngine neither declares nor assigns a
// ledger of its own, and reaches the ledger only as the address of the
// Service's existing field — pinned structurally so a later refactor that
// gives the rebuild path a ledger of its own fails loudly, rather than
// silently restoring "since the last Set* call". Mirrors the AST-walking
// style of agent_error_fire_site_pin_test.go.
func TestInitWorkflowEngineReachesLedgerOnlyByFieldAddress(t *testing.T) {
	root, err := findAgentErrorBackendSourceRoot(".")
	if err != nil {
		t.Fatalf("locate backend source root: %v", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(root, "internal", "orchestrator", "service.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse service.go: %v", err)
	}

	var fn *ast.FuncDecl
	ast.Inspect(file, func(n ast.Node) bool {
		decl, ok := n.(*ast.FuncDecl)
		if !ok || decl.Name.Name != "initWorkflowEngine" {
			return true
		}
		fn = decl
		return false
	})
	if fn == nil || fn.Body == nil {
		t.Fatal("initWorkflowEngine not found in service.go")
	}

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if stmt.Tok != token.DEFINE {
				return true
			}
			for _, lhs := range stmt.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && strings.Contains(strings.ToLower(ident.Name), "ledger") {
					t.Errorf("initWorkflowEngine declares its own %q instead of reusing the Service field", ident.Name)
				}
			}
		case *ast.GenDecl:
			if stmt.Tok != token.VAR {
				return true
			}
			for _, spec := range stmt.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range valueSpec.Names {
					if strings.Contains(strings.ToLower(name.Name), "ledger") {
						t.Errorf("initWorkflowEngine declares var %q instead of reusing the Service field", name.Name)
					}
				}
			}
		}
		return true
	})

	foundFieldAddress := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "newWorkflowStore" {
			return true
		}
		for _, arg := range call.Args {
			unary, ok := arg.(*ast.UnaryExpr)
			if !ok || unary.Op != token.AND {
				continue
			}
			sel, ok := unary.X.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "operationLedger" {
				continue
			}
			if recv, ok := sel.X.(*ast.Ident); ok && recv.Name == "s" {
				foundFieldAddress = true
			}
		}
		return true
	})
	if !foundFieldAddress {
		t.Fatal("initWorkflowEngine does not pass &s.operationLedger to newWorkflowStore")
	}
}
