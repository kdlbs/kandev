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
