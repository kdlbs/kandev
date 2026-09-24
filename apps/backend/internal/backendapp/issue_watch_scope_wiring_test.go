package backendapp

import (
	"fmt"
	"go/ast"
	"testing"
)

// TestSentryAndGitLabListAllIssueWatchesScopingIsWired guards the two lines
// that make ListAllIssueWatches' per-user workspace filter real in the
// shipped binary. internal/sentry and internal/gitlab both default to
// unscoped (nil workspaceAuthorizer) so their own package tests stay green
// with a hand-injected fake — only this wiring block makes the real
// taskSvc.AuthorizeWorkspaceAccess check apply, so a rebase or refactor that
// drops one of these lines would leak every workspace's watch config again
// (see docs/plans/alert-ingest/task-00-issue-watch-workspace-scoping.md) with
// no other failing test.
func TestSentryAndGitLabListAllIssueWatchesScopingIsWired(t *testing.T) {
	fn := findFuncDecl(t, "helpers.go", "registerRoutes")
	want := map[string]bool{
		"p.services.GitLab.SetWorkspaceAuthorizer": false,
		"p.services.Sentry.SetWorkspaceAuthorizer": false,
	}
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		callee := dottedExprString(call.Fun)
		if _, tracked := want[callee]; !tracked {
			return true
		}
		if dottedExprString(call.Args[0]) == "p.taskSvc.AuthorizeWorkspaceAccess" {
			want[callee] = true
		}
		return true
	})

	for setter, wired := range want {
		if !wired {
			t.Errorf("registerRoutes never calls %s(p.taskSvc.AuthorizeWorkspaceAccess); without it "+
				"ListAllIssueWatches leaks every workspace's watch config to a scoped caller", setter)
		}
	}
}

// dottedExprString renders a chain of selectors/idents (e.g. p.services.GitLab
// or p.taskSvc.AuthorizeWorkspaceAccess) as a dotted string for comparison.
func dottedExprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return dottedExprString(v.X) + "." + v.Sel.Name
	case *ast.Ident:
		return v.Name
	default:
		return fmt.Sprintf("%T", e)
	}
}
