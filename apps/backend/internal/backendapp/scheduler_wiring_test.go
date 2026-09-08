package backendapp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestGlobalSchedulingStartsOutsideOfficeFeatureGate(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	var callsInOfficeInit, callsElsewhere int
	var foundOfficeInit bool
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		count := countNamedCalls(fn, "startSchedulingRuntime")
		if fn.Name.Name == "initOfficeServices" {
			foundOfficeInit = true
			callsInOfficeInit += count
			continue
		}
		callsElsewhere += count
	}

	if !foundOfficeInit {
		t.Fatal("initOfficeServices not found in main.go; re-point this guard at the Office-gated function")
	}
	if callsInOfficeInit > 0 {
		t.Errorf("startSchedulingRuntime is called inside the Office-gated initializer %d time(s)", callsInOfficeInit)
	}
	if callsElsewhere == 0 {
		t.Error("startSchedulingRuntime is not called outside the Office feature gate")
	}
}

// TestRoutinesHandlerNotWiredWithTypedNilService guards the Office-disabled
// cron panic regression. startCronScheduler must gate the routines service
// behind a nil check before assigning it into the RoutineTicker interface;
// passing the raw *RoutineService parameter (routineSvc) straight into
// NewRoutinesHandler produces a typed-nil interface that panics every tick
// when Office is disabled.
func TestRoutinesHandlerNotWiredWithTypedNilService(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "cron.go", nil, 0)
	if err != nil {
		t.Fatalf("parse cron.go: %v", err)
	}

	var startFn *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "startCronScheduler" {
			startFn = fn
			break
		}
	}
	if startFn == nil {
		t.Fatal("startCronScheduler not found in cron.go")
	}

	var passesRawService bool
	ast.Inspect(startFn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "NewRoutinesHandler" || len(call.Args) == 0 {
			return true
		}
		if ident, ok := call.Args[0].(*ast.Ident); ok && ident.Name == "routineSvc" {
			passesRawService = true
		}
		return true
	})

	if passesRawService {
		t.Error("startCronScheduler passes the raw routineSvc pointer into NewRoutinesHandler; " +
			"gate it behind a nil check via a RoutineTicker interface variable to avoid the typed-nil panic")
	}
}

// TestParentWakeReconcilerWiredWhenOfficeEnabled guards ParentWakeReconciler's
// registration end to end: the service tests (scheduler_wake_reconciler_test.go)
// only exercise the handler directly, so nothing proves Office-enabled startup
// actually constructs it and hands it to the cron loop rather than leaving the
// typed-nil schedulercron.Handler the "Office disabled" path relies on.
func TestParentWakeReconcilerWiredWhenOfficeEnabled(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	var startFn *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "startSchedulingRuntime" {
			startFn = fn
			break
		}
	}
	if startFn == nil {
		t.Fatal("startSchedulingRuntime not found in main.go")
	}

	var constructedUnderOfficeGate bool
	ast.Inspect(startFn, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok || !isServicesOfficeNotNil(ifStmt.Cond) {
			return true
		}
		ast.Inspect(ifStmt.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, lhs := range assign.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident.Name != "parentWakeReconciler" || i >= len(assign.Rhs) {
					continue
				}
				if isSelectorCall(assign.Rhs[i], "NewParentWakeReconciler") {
					constructedUnderOfficeGate = true
				}
			}
			return true
		})
		return false
	})
	if !constructedUnderOfficeGate {
		t.Error("parentWakeReconciler is not constructed via officeservice.NewParentWakeReconciler " +
			"inside the `services.Office != nil` gate; ParentWakeReconciler would never run")
	}

	var passedToCronLoop bool
	ast.Inspect(startFn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "startCronScheduler" {
			return true
		}
		for _, arg := range call.Args {
			if a, ok := arg.(*ast.Ident); ok && a.Name == "parentWakeReconciler" {
				passedToCronLoop = true
			}
		}
		return true
	})
	if !passedToCronLoop {
		t.Error("parentWakeReconciler is not passed as an argument to startCronScheduler")
	}
}

// TestParentWakeReconcilerRegisteredInCronLoop guards the second half of the
// same registration chain: startCronScheduler must actually forward its
// parentWakeReconciler parameter into the loop it builds, not just accept and
// drop it.
func TestParentWakeReconcilerRegisteredInCronLoop(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "cron.go", nil, 0)
	if err != nil {
		t.Fatalf("parse cron.go: %v", err)
	}

	var startFn *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "startCronScheduler" {
			startFn = fn
			break
		}
	}
	if startFn == nil {
		t.Fatal("startCronScheduler not found in cron.go")
	}

	var hasParam bool
	for _, field := range startFn.Type.Params.List {
		for _, name := range field.Names {
			if name.Name == "parentWakeReconciler" {
				hasParam = true
			}
		}
	}
	if !hasParam {
		t.Fatal("startCronScheduler no longer takes a parentWakeReconciler parameter")
	}

	var passedToNewLoop bool
	ast.Inspect(startFn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !isSelectorCall(call, "NewLoop") {
			return true
		}
		for _, arg := range call.Args {
			if a, ok := arg.(*ast.Ident); ok && a.Name == "parentWakeReconciler" {
				passedToNewLoop = true
			}
		}
		return true
	})
	if !passedToNewLoop {
		t.Error("startCronScheduler does not pass parentWakeReconciler into schedulercron.NewLoop; " +
			"it would be accepted but never ticked")
	}
}

// isServicesOfficeNotNil matches the `services.Office != nil` guard condition
// as an AST shape, without depending on operand identifier names beyond that.
func isServicesOfficeNotNil(cond ast.Expr) bool {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ {
		return false
	}
	sel, ok := bin.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Office" {
		return false
	}
	ident, ok := bin.Y.(*ast.Ident)
	return ok && ident.Name == "nil"
}

// isSelectorCall reports whether expr is a call whose function is a
// package-qualified selector ending in name, e.g. officeservice.NewX(...) or
// schedulercron.NewLoop(...).
func isSelectorCall(expr ast.Expr, name string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name
}

func countNamedCalls(fn *ast.FuncDecl, name string) int {
	count := 0
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if ok && ident.Name == name {
			count++
		}
		return true
	})
	return count
}
