package backendapp

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestWorkflowAccessCheckersAreWired guards the three lines that make the
// workflow-step surface's per-user scoping real in the shipped binary.
//
// The workflow package cannot resolve a workflow's owner on its own, so its
// guards call out through checkers that default to open when nothing is wired
// (the same shape SetSessionAccessChecker has always had, so a Service built
// without the task domain still works). Every test in internal/workflow wires
// its own fake, so deleting one of these lines here leaves that whole suite
// green while the real backend hands one user another user's workflow steps
// again — the defect the guards exist to close, reintroduced with no failing
// test. A rebase resolving a conflict in this DI function by dropping a line
// is exactly how that would happen.
//
// The wiring lives in wireTaskWorkflowCrossReferences, which provideServices
// calls unconditionally with the real taskSvc/workflowSvc — inspecting that
// function directly gives the same protection as inspecting provideServices
// itself would.
func TestWorkflowAccessCheckersAreWired(t *testing.T) {
	want := map[string]string{
		"SetWorkflowAccessChecker":  "AuthorizeWorkflowAccess",
		"SetWorkspaceAccessChecker": "AuthorizeWorkspaceAccess",
		"SetTaskAccessChecker":      "AuthorizeTaskAccess",
		"SetSessionAccessChecker":   "AuthorizeSessionAccess",
	}

	wireFn := findFuncDecl(t, "services.go", "wireTaskWorkflowCrossReferences")
	got := map[string]string{}
	ast.Inspect(wireFn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		if _, tracked := want[sel.Sel.Name]; !tracked {
			return true
		}
		// Only the workflow service's wiring counts; the orchestrator's
		// identically-named setters are guarded by their own test.
		if receiver, ok := sel.X.(*ast.Ident); !ok || receiver.Name != "workflowSvc" {
			return true
		}
		got[sel.Sel.Name] = argumentSelector(call.Args[0])
		return true
	})

	for setter, checker := range want {
		switch arg, called := got[setter]; {
		case !called:
			t.Errorf("wireTaskWorkflowCrossReferences never calls workflowSvc.%s; without it the workflow-step "+
				"surface falls open to any authenticated user", setter)
		case arg != "taskSvc."+checker:
			t.Errorf("workflowSvc.%s(%s), want taskSvc.%s", setter, arg, checker)
		}
	}

	if !callsFunction(t, "services.go", "provideServices", "wireTaskWorkflowCrossReferences") {
		t.Error("provideServices never calls wireTaskWorkflowCrossReferences; the workflow-step " +
			"surface falls open to any authenticated user")
	}
}

// callsFunction reports whether callerName's body contains a call to a
// function literally named calleeName (bare identifier, not a method).
func callsFunction(t *testing.T, filename, callerName, calleeName string) bool {
	t.Helper()
	callerFn := findFuncDecl(t, filename, callerName)
	found := false
	ast.Inspect(callerFn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == calleeName {
			found = true
		}
		return true
	})
	return found
}

// findFuncDecl parses filename and returns the named top-level function.
func findFuncDecl(t *testing.T, filename, name string) *ast.FuncDecl {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn
		}
	}
	t.Fatalf("%s not found in %s; re-point this guard at the DI function", name, filename)
	return nil
}

// argumentSelector renders a `receiver.Method` argument for comparison.
func argumentSelector(arg ast.Expr) string {
	sel, ok := arg.(*ast.SelectorExpr)
	if !ok {
		return fmt.Sprintf("%T", arg)
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return fmt.Sprintf("%T.%s", sel.X, sel.Sel.Name)
	}
	return ident.Name + "." + sel.Sel.Name
}
