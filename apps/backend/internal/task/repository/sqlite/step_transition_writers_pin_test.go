package sqlite

// TestStepTransitionWritersArePinned is the writer-health control that
// matters most, per the spec: the realistic failure mode is not the ledger
// writer breaking, it's a NEW production statement mutating
// tasks.workflow_step_id that nobody wired into it. This walks the package's
// own source (go/parser, not a line regex — every statement here is a
// multi-line raw string literal, which a regex over raw lines misreports on)
// and asserts the exact set of functions containing such a statement matches
// the registered, ledger-wired set. Add a new mutating statement and this
// test fails until it is registered below AND wired to recordStepTransition.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// registeredStepMutators is the closed set of functions in this package
// known to mutate tasks.workflow_step_id, each already wired to
// recordStepTransition. A new function containing a matching statement that
// is not in this set means either: (a) it is a genuine new step-mutation
// path — wire it to recordStepTransition and add it here; or (b) it is a
// false positive (e.g. a table-rebuild migration copying existing rows
// verbatim) — narrow the detection below rather than registering it.
var registeredStepMutators = []string{
	"insertTaskTx",
	"updateTaskTx",
	"UpdateTaskIfWorkflowStepHasCapacity",
	"PromoteQueuedTaskIfWorkflowStepHasCapacity",
	"RestoreTaskMessageRollbackIfSessionState",
	"AddTaskToWorkflow",
	"RemoveTaskFromWorkflow",
}

func TestStepTransitionWritersArePinned(t *testing.T) {
	found, err := findWorkflowStepIDMutators(t, ".")
	if err != nil {
		t.Fatalf("scan package for workflow_step_id mutators: %v", err)
	}

	registered := make(map[string]bool, len(registeredStepMutators))
	for _, name := range registeredStepMutators {
		registered[name] = true
	}

	var unregistered []string
	for name := range found {
		if !registered[name] {
			unregistered = append(unregistered, name)
		}
	}
	if len(unregistered) > 0 {
		sort.Strings(unregistered)
		t.Fatalf(
			"function(s) %v contain a statement that mutates tasks.workflow_step_id "+
				"but are not registered in registeredStepMutators. If this is a genuine "+
				"new step-mutation path: wire it to (*Repository).recordStepTransition "+
				"inside its own transaction (see step_transitions.go), then add its name "+
				"to registeredStepMutators in this file. If this is a false positive "+
				"(e.g. a migration copying rows verbatim), narrow the detection in "+
				"findWorkflowStepIDMutators instead of registering it.",
			unregistered)
	}

	var missing []string
	for _, name := range registeredStepMutators {
		if !found[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf(
			"registeredStepMutators lists %v but no matching statement was found in "+
				"this package — the function was likely renamed, removed, or its SQL "+
				"literal changed shape. Update registeredStepMutators to match reality.",
			missing)
	}
}

// findWorkflowStepIDMutators parses every non-test .go file directly in dir
// and returns the set of top-level function names whose body contains a
// string literal that either inserts a new tasks row with an explicit
// workflow_step_id column, or updates the tasks table and assigns
// workflow_step_id.
func findWorkflowStepIDMutators(t *testing.T, dir string) (map[string]bool, error) {
	t.Helper()
	fset := token.NewFileSet()
	found := make(map[string]bool)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, err
		}
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			if functionMutatesWorkflowStepID(fn.Body) {
				found[fn.Name.Name] = true
			}
			return true
		})
	}
	return found, nil
}

func functionMutatesWorkflowStepID(body *ast.BlockStmt) bool {
	mutates := false
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if literalMutatesWorkflowStepID(strings.Trim(lit.Value, "`\"")) {
			mutates = true
		}
		return true
	})
	return mutates
}

func literalMutatesWorkflowStepID(sql string) bool {
	insertsWithColumn := strings.Contains(sql, "INSERT INTO tasks (") && strings.Contains(sql, "workflow_step_id")
	// "workflow_step_id = " (not just "= ?") also catches an UPDATE that
	// assigns a literal value directly, e.g. RemoveTaskFromWorkflow's
	// "workflow_step_id = ''" — a parameter-only pattern would miss it.
	updatesColumn := strings.Contains(sql, "UPDATE tasks") && strings.Contains(sql, "workflow_step_id = ")
	return insertsWithColumn || updatesColumn
}
