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
