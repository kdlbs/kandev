package backendapp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestSetWriteDepsIsWiredOutsideOfficeGate pins a startup-ordering invariant
// that no unit test on an individual helper can catch, because the bug is the
// *placement* of a call rather than the behavior of any one function.
//
// Plugins now start whenever services.Plugins is non-nil (they used to be
// gated on features.plugins, which shipped off in production). initOfficeServices
// returns early when features.office is false — also the production default —
// so any plugin wiring left inside that function silently never runs on a
// default production backend. For Host write deps that means
// Host.Messages().Send returns Unimplemented and CreateTask's start_agent is a
// no-op, with no error anywhere: exactly the kind of failure that shows up as
// "the plugin just doesn't do anything".
//
// Guarding it structurally keeps the invariant honest without booting the whole
// backend: SetWriteDeps must be called from somewhere that features.office does
// not gate.
func TestSetWriteDepsIsWiredOutsideOfficeGate(t *testing.T) {
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
		count := countSetWriteDepsCalls(fn)
		if fn.Name.Name == "initOfficeServices" {
			foundOfficeInit = true
			callsInOfficeInit += count
			continue
		}
		callsElsewhere += count
	}

	// Without this the test degrades silently: rename or move
	// initOfficeServices and every assertion below still "passes" while
	// guarding nothing. Fail loudly instead so whoever renames it has to
	// re-point the guard at the Office-gated function.
	if !foundOfficeInit {
		t.Fatal("initOfficeServices not found in main.go; re-point this guard at the Office-gated function")
	}

	if callsInOfficeInit > 0 {
		t.Errorf(
			"SetWriteDeps is called %d time(s) inside initOfficeServices, which returns early when "+
				"features.office=false (the production default). Plugins start regardless of that flag, so the "+
				"write deps would never be wired on a default production backend: Host.Messages().Send would "+
				"return Unimplemented and CreateTask start_agent would silently no-op. Wire it outside the Office gate.",
			callsInOfficeInit,
		)
	}
	if callsElsewhere == 0 {
		t.Error("no SetWriteDeps call found outside initOfficeServices; plugin Host write deps would never be wired")
	}
}

// countSetWriteDepsCalls reports how many `<expr>.SetWriteDeps(...)` calls the
// function body contains.
func countSetWriteDepsCalls(fn *ast.FuncDecl) int {
	count := 0
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "SetWriteDeps" {
			count++
		}
		return true
	})
	return count
}
