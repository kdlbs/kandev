package backendapp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestOfficeActivationRegistersBlockerSubscribers guards that Office
// activation subscribes both the blocker success bridge and the blocker
// failure escalation, and fails activation when either subscription fails.
// The handlers themselves are covered by the office service tests.
func TestOfficeActivationRegistersBlockerSubscribers(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	var activate *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "activateOfficeServices" {
			activate = fn
		}
	}
	if activate == nil {
		t.Fatal("activateOfficeServices not found in main.go")
	}
	for _, name := range []string{"RegisterTaskCompletionSubscriber", "RegisterBlockerFailureSubscriber"} {
		if !registrationFailsActivation(activate, name) {
			t.Errorf("activateOfficeServices does not call %s inside an `if err := ...; err != nil { return false }` guard", name)
		}
	}
}

func registrationFailsActivation(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok || ifStmt.Init == nil {
			return true
		}
		assign, ok := ifStmt.Init.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 || !isSelectorCall(assign.Rhs[0], name) {
			return true
		}
		for _, stmt := range ifStmt.Body.List {
			ret, ok := stmt.(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				continue
			}
			if ident, ok := ret.Results[0].(*ast.Ident); ok && ident.Name == "false" {
				found = true
			}
		}
		return true
	})
	return found
}
