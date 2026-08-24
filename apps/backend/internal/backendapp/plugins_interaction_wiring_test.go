package backendapp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetInteractionResponderIsWiredOutsideOfficeGate pins the same
// startup-placement invariant TestSetWriteDepsIsWiredOutsideOfficeGate does,
// for the interaction write path. The failure mode it guards is silent:
// plugins start whenever services.Plugins is non-nil, while initOfficeServices
// returns early when features.office is false (the production default). Wiring
// the responder there would leave every default backend answering
// Unimplemented to RespondToPermission / AnswerClarification / Cancel with no
// error anywhere upstream — "the plugin just doesn't do anything".
//
// Unlike the write-deps guard this scans the whole package rather than
// main.go, because the responder is wired where the clarification handler is
// constructed.
func TestSetInteractionResponderIsWiredOutsideOfficeGate(t *testing.T) {
	const wiringCall = "SetInteractionResponder"
	const officeGate = "initOfficeServices"

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	var callsInOfficeInit, callsElsewhere int
	var foundOfficeInit bool
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			count := countMethodCalls(fn, wiringCall)
			if fn.Name.Name == officeGate {
				foundOfficeInit = true
				callsInOfficeInit += count
				continue
			}
			callsElsewhere += count
		}
	}

	// Without this the guard degrades silently: rename or move the
	// Office-gated function and every assertion below still "passes" while
	// guarding nothing.
	if !foundOfficeInit {
		t.Fatalf("%s not found in package backendapp; re-point this guard at the Office-gated function", officeGate)
	}
	if callsInOfficeInit > 0 {
		t.Errorf(
			"%s is called %d time(s) inside %s, which returns early when features.office=false (the "+
				"production default). Plugins start regardless of that flag, so the interaction write path "+
				"would never be wired on a default production backend. Wire it outside the Office gate.",
			wiringCall, callsInOfficeInit, officeGate,
		)
	}
	if callsElsewhere == 0 {
		t.Errorf("no %s call found outside %s; the plugin Host interaction writes would never be wired", wiringCall, officeGate)
	}
}

// countMethodCalls reports how many `<expr>.<method>(...)` calls the function
// body contains.
func countMethodCalls(fn *ast.FuncDecl, method string) int {
	count := 0
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == method {
			count++
		}
		return true
	})
	return count
}
