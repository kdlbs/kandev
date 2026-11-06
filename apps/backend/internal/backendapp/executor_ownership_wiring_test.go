package backendapp

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"testing"
)

// Provider unit tests install their own policy. Pin the production composition
// so no scheduler silently retains an unwired identity boundary.
func TestExecutorOwnershipAutonomousPoliciesAreWired(t *testing.T) {
	want := map[string]bool{"services.GitHub": false, "services.GitLab": false, "services.Jira": false, "services.Linear": false, "services.Sentry": false, "services.AzureDevOps": false, "services.Automation.Service": false}
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "SetExecutorContext" {
			return true
		}
		policy, ok := call.Args[0].(*ast.SelectorExpr)
		if !ok || policy.Sel.Name != "PrepareAutonomousExecutor" {
			return true
		}
		var receiver bytes.Buffer
		if err := format.Node(&receiver, files, selector.X); err != nil {
			t.Fatal(err)
		}
		if _, tracked := want[receiver.String()]; tracked {
			want[receiver.String()] = true
		}
		return true
	})
	for source, wired := range want {
		if !wired {
			t.Errorf("%s is missing current-creator executor admission", source)
		}
	}
}
