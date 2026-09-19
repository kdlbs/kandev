package backendapp

import (
	"go/ast"
	"testing"
)

func TestProvideOrchestratorWiresStreamingMessageRetraction(t *testing.T) {
	provideFn := findFuncDecl(t, "orchestrator.go", "provideOrchestrator")
	wired := false
	ast.Inspect(provideFn, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "SetStreamingMessageRetractionService" {
			return true
		}
		receiver, receiverOK := selector.X.(*ast.Ident)
		argument, argumentOK := call.Args[0].(*ast.Ident)
		wired = receiverOK && receiver.Name == "orchestratorSvc" &&
			argumentOK && argument.Name == "taskSvc"
		return true
	})
	if !wired {
		t.Fatal("provideOrchestrator does not wire taskSvc as the streaming message retraction service")
	}
}
