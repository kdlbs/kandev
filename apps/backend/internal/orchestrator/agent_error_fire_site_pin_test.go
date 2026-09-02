package orchestrator

// TestOnAgentErrorFireSitesArePinned is the regression guard for the defect
// this card fixes: on_agent_error declared, compiled, and silently never
// dispatched outside Office. It walks the whole backend source tree
// (go/parser, rooted at apps/backend/internal so a third fire site added in
// ANY package — not just this one — can fail it) and asserts the set of
// functions that actually FIRE engine.TriggerOnAgentError is exactly the two
// registered ones. A bare identifier scan would also match the trigger
// constant declaration, the compileGenericActions trigger-map entry, the
// OnAgentErrorPayload doc comment, and the Office engine dispatcher's
// session-resolution branches — so a fire site is defined syntactically: an
// occurrence in the value of the `Trigger:` key of an engine.HandleInput
// composite literal (the orchestrator shape), or a direct call argument to
// dispatchEngineTrigger (the Office shape, which passes the trigger
// positionally). Mirrors the design of
// task/repository/sqlite/step_transition_writers_pin_test.go.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// registeredAgentErrorFireSites is the closed set of functions known to fire
// engine.TriggerOnAgentError, keyed "packageRelDir/ReceiverType.FuncName"
// (relative to the apps/backend/internal scan root). A builder who renames
// either function must update this set in the same commit.
var registeredAgentErrorFireSites = []string{
	"office/service/Service.dispatchAgentErrorTrigger",
	"orchestrator/Service.dispatchKanbanAgentErrorTrigger",
}

func TestOnAgentErrorFireSitesArePinned(t *testing.T) {
	root, err := findAgentErrorBackendSourceRoot(".")
	if err != nil {
		t.Fatalf("locate backend source root: %v", err)
	}
	if filepath.Base(root) != "internal" {
		t.Fatalf("scan root %q is not the backend internal/ directory", root)
	}
	if _, statErr := os.Stat(filepath.Join(root, "office")); statErr != nil {
		t.Fatalf("scan root %q does not contain sibling package internal/office: %v", root, statErr)
	}

	found, err := findAgentErrorFireSites(root)
	if err != nil {
		t.Fatalf("scan backend source for on_agent_error fire sites: %v", err)
	}

	registered := make(map[string]bool, len(registeredAgentErrorFireSites))
	for _, name := range registeredAgentErrorFireSites {
		registered[name] = true
	}

	var unregistered []string
	for name := range found {
		if !registered[name] {
			unregistered = append(unregistered, name)
		}
	}
	if len(unregistered) > 0 {
		sort.Strings(unregistered)
		t.Fatalf(
			"function(s) %v fire engine.TriggerOnAgentError but are not in "+
				"registeredAgentErrorFireSites. A third fire site is a regression "+
				"of this card's own defect (double dispatch) unless it replaces one "+
				"of the two registered sites — update the registered set only if "+
				"that is genuinely what happened.",
			unregistered)
	}

	var missing []string
	for _, name := range registeredAgentErrorFireSites {
		if !found[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf(
			"registeredAgentErrorFireSites lists %v but no matching fire site was "+
				"found — the function was likely renamed or removed, or on_agent_error "+
				"dispatch was removed from it (the exact defect this card fixes). "+
				"Update registeredAgentErrorFireSites to match reality.",
			missing)
	}
}

func findAgentErrorBackendSourceRoot(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return filepath.Join(dir, "internal"), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod found above %s", startDir)
		}
		dir = parent
	}
}

func findAgentErrorFireSites(root string) (map[string]bool, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == "testdata" && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		if strings.HasSuffix(name, ".go") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	found := make(map[string]bool)
	fset := token.NewFileSet()
	for _, path := range paths {
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil, parseErr
		}
		relDir, relErr := filepath.Rel(root, filepath.Dir(path))
		if relErr != nil {
			return nil, relErr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			if functionFiresTriggerOnAgentError(fn.Body) {
				found[agentErrorFuncIdentity(fn, relDir)] = true
			}
			return true
		})
	}
	return found, nil
}

// functionFiresTriggerOnAgentError reports whether body contains an
// occurrence of TriggerOnAgentError in one of the two fire-site shapes.
func functionFiresTriggerOnAgentError(body *ast.BlockStmt) bool {
	fires := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.CompositeLit:
			sel, ok := v.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "HandleInput" {
				return true
			}
			for _, elt := range v.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "Trigger" {
					continue
				}
				if identIsTriggerOnAgentError(kv.Value) {
					fires = true
				}
			}
		case *ast.CallExpr:
			sel, ok := v.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "dispatchEngineTrigger" {
				return true
			}
			for _, arg := range v.Args {
				if identIsTriggerOnAgentError(arg) {
					fires = true
				}
			}
		}
		return true
	})
	return fires
}

func identIsTriggerOnAgentError(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.SelectorExpr:
		return v.Sel.Name == "TriggerOnAgentError"
	case *ast.Ident:
		return v.Name == "TriggerOnAgentError"
	default:
		return false
	}
}

func agentErrorFuncIdentity(fn *ast.FuncDecl, relDir string) string {
	name := fn.Name.Name
	if recv := agentErrorReceiverTypeName(fn); recv != "" {
		name = recv + "." + name
	}
	if relDir == "" || relDir == "." {
		return name
	}
	return filepath.ToSlash(relDir) + "/" + name
}

func agentErrorReceiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}
