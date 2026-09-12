package backendapp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestAgentFamilyResolverIsWired guards the one line that makes per-step
// configure_session model selection work in the real binary.
//
// Every orchestrator test installs a resolver straight into the Service struct,
// so deleting SetAgentFamilyResolver from provideOrchestrator leaves that whole
// suite green while the shipped backend silently falls back to comparing a
// hand-written agent family ("Claude") against the canonical ID a session stores
// ("claude-acp"). Nothing matches, no rule applies, and every workflow step runs
// on the task profile's model again — the defect this wiring exists to fix,
// reintroduced with no failing test.
func TestAgentFamilyResolverIsWired(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "orchestrator.go", nil, 0)
	if err != nil {
		t.Fatalf("parse orchestrator.go: %v", err)
	}

	var provideFn *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "provideOrchestrator" {
			provideFn = fn
			break
		}
	}
	if provideFn == nil {
		t.Fatal("provideOrchestrator not found in orchestrator.go; re-point this guard at the DI function")
	}

	var resolverArg string
	var calls int
	ast.Inspect(provideFn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "SetAgentFamilyResolver" || len(call.Args) != 1 {
			return true
		}
		calls++
		if ident, ok := call.Args[0].(*ast.Ident); ok {
			resolverArg = ident.Name
		}
		return true
	})

	if calls == 0 {
		t.Fatal("provideOrchestrator never calls SetAgentFamilyResolver; " +
			"configure_session rules cannot resolve an agent family without it, so per-step " +
			"model selection silently stops working")
	}
	if calls > 1 {
		t.Errorf("SetAgentFamilyResolver is called %d times in provideOrchestrator; expected exactly one wiring site", calls)
	}
	// The agent registry is the only collaborator that knows every installed
	// agent, including the custom TUI agents a user registers at runtime.
	if resolverArg != "agentRegistry" {
		t.Errorf("SetAgentFamilyResolver is wired with %q, want the agent registry (agentRegistry)", resolverArg)
	}
}

func TestOfficeCEOResolverAlsoWiresCoordinatorProfileResolver(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	var wireFn *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "wireWorkflowEngineForOffice" {
			wireFn = fn
			break
		}
	}
	if wireFn == nil {
		t.Fatal("wireWorkflowEngineForOffice not found in main.go; re-point this guard at the Office composition function")
	}

	ceoVars := map[string]bool{}
	setterArgs := map[string]string{}
	ast.Inspect(wireFn, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if ok && len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
			if ident, ok := assign.Lhs[0].(*ast.Ident); ok && callsNewCEOAgentAdapter(assign.Rhs[0]) {
				ceoVars[ident.Name] = true
			}
		}

		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "SetEngineCEOResolver", "SetCoordinatorProfileResolver":
			if ident, ok := call.Args[0].(*ast.Ident); ok {
				setterArgs[sel.Sel.Name] = ident.Name
			}
		}
		return true
	})

	engineArg := setterArgs["SetEngineCEOResolver"]
	coordinatorArg := setterArgs["SetCoordinatorProfileResolver"]
	if engineArg == "" {
		t.Fatal("wireWorkflowEngineForOffice never calls SetEngineCEOResolver")
	}
	if coordinatorArg == "" {
		t.Fatal("wireWorkflowEngineForOffice never calls SetCoordinatorProfileResolver; Office Coordinator MCP sessions will not receive the attested workspace task-tree capability")
	}
	if engineArg != coordinatorArg {
		t.Fatalf("CEO resolver mismatch: SetEngineCEOResolver(%q), SetCoordinatorProfileResolver(%q)", engineArg, coordinatorArg)
	}
	if !ceoVars[coordinatorArg] {
		t.Fatalf("SetCoordinatorProfileResolver uses %q, want the Office CEO adapter created by NewCEOAgentAdapter", coordinatorArg)
	}
}

func callsNewCEOAgentAdapter(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "NewCEOAgentAdapter"
}
