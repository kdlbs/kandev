package orchestrator

// TestEvaluateOnlyOperationMarkingCallSitesArePinned is the AC-EO-15
// regression guard for docs/specs/workflow-evaluate-only-operation-marking:
// it walks the whole backend source tree (go/parser, rooted at apps/backend)
// for engine.HandleInput composite literals that pair EvaluateOnly: true with
// a non-empty OperationID — the exact shape this spec's contract governs.
// Every such call site must capture HandleResult.OperationMarkDeferred and
// mark the operation applied itself only after its own commit succeeds
// (AC-EO-10, AC-EO-13); the closed set here is the only place that
// responsibility currently lives. A new call site pairing the two fields is
// not automatically wrong, but it must take on that same responsibility
// before being added to the registered set. Mirrors the design of
// agent_error_fire_site_pin_test.go, reusing its shared AST-walk helpers.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var registeredEvaluateOnlyOperationMarkingSites = []string{
	"internal/orchestrator/Service.dispatchKanbanAgentErrorTrigger",
}

func TestEvaluateOnlyOperationMarkingCallSitesArePinned(t *testing.T) {
	root, err := findAgentErrorBackendSourceRoot(".")
	if err != nil {
		t.Fatalf("locate backend source root: %v", err)
	}
	if filepath.Base(root) != "backend" {
		t.Fatalf("scan root %q is not the apps/backend directory", root)
	}

	found, err := findEvaluateOnlyOperationMarkingSites(root)
	if err != nil {
		t.Fatalf("scan backend source for EvaluateOnly+OperationID call sites: %v", err)
	}

	registered := make(map[string]bool, len(registeredEvaluateOnlyOperationMarkingSites))
	for _, name := range registeredEvaluateOnlyOperationMarkingSites {
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
			"function(s) %v pair engine.HandleInput{EvaluateOnly: true} with a "+
				"non-empty OperationID but are not in "+
				"registeredEvaluateOnlyOperationMarkingSites. Every such call site "+
				"must capture HandleResult.OperationMarkDeferred and mark the "+
				"operation applied itself only after its own commit succeeds "+
				"(AC-EO-10, AC-EO-13, AC-EO-15) — update the registered set only "+
				"after confirming the new call site does that.",
			unregistered)
	}

	var missing []string
	for _, name := range registeredEvaluateOnlyOperationMarkingSites {
		if !found[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf(
			"registeredEvaluateOnlyOperationMarkingSites lists %v but no matching "+
				"call site was found — it was likely renamed, removed, or no longer "+
				"pairs EvaluateOnly with a non-empty OperationID. Update "+
				"registeredEvaluateOnlyOperationMarkingSites to match reality.",
			missing)
	}
}

// TestFunctionPairsEvaluateOnlyWithOperationID_UnrelatedHandleInputType pins
// the false-positive fix: a HandleInput composite literal from a package
// other than internal/workflow/engine must not register as a call site this
// spec governs, even though it pairs the same two field names.
func TestFunctionPairsEvaluateOnlyWithOperationID_UnrelatedHandleInputType(t *testing.T) {
	const src = `package other

import unrelated "some/other/package"

func Call() {
	unrelated.HandleInput{EvaluateOnly: true, OperationID: "x"}
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "unrelated.go", src, 0)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	engineAlias := engineImportAlias(file)
	isEnginePackage := file.Name != nil && file.Name.Name == "engine"

	var fn *ast.FuncDecl
	ast.Inspect(file, func(n ast.Node) bool {
		if decl, ok := n.(*ast.FuncDecl); ok {
			fn = decl
			return false
		}
		return true
	})
	if fn == nil || fn.Body == nil {
		t.Fatalf("fixture did not parse a function body")
	}

	if functionPairsEvaluateOnlyWithOperationID(fn.Body, engineAlias, isEnginePackage) {
		t.Fatalf("unrelated package's HandleInput{EvaluateOnly, OperationID} literal must not match")
	}
}

func findEvaluateOnlyOperationMarkingSites(root string) (map[string]bool, error) {
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
		engineAlias := engineImportAlias(file)
		isEnginePackage := file.Name != nil && file.Name.Name == "engine"
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			if functionPairsEvaluateOnlyWithOperationID(fn.Body, engineAlias, isEnginePackage) {
				found[agentErrorFuncIdentity(fn, relDir)] = true
			}
			return true
		})
	}
	return found, nil
}

// engineImportAlias returns the local qualifier a file uses to refer to
// internal/workflow/engine ("" if the file does not import it), resolving a
// renamed import instead of assuming the package name "engine".
func engineImportAlias(file *ast.File) string {
	const enginePath = "github.com/kandev/kandev/internal/workflow/engine"
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != enginePath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "engine"
	}
	return ""
}

// functionPairsEvaluateOnlyWithOperationID reports whether body contains a
// HandleInput composite literal resolved to internal/workflow/engine — either
// qualified via engineAlias (external callers) or bare when the file itself
// is package engine (isEnginePackage; spec.md's detection predicate names
// both forms) — with both an EvaluateOnly: true key and an OperationID key
// whose value is not the empty-string literal. A HandleInput literal from an
// unrelated package is not a match.
func functionPairsEvaluateOnlyWithOperationID(body *ast.BlockStmt, engineAlias string, isEnginePackage bool) bool {
	pairs := false
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if !isHandleInputLitType(lit.Type, engineAlias, isEnginePackage) {
			return true
		}
		hasEvaluateOnlyTrue := false
		hasNonEmptyOperationID := false
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch key.Name {
			case "EvaluateOnly":
				if ident, ok := kv.Value.(*ast.Ident); ok && ident.Name == "true" {
					hasEvaluateOnlyTrue = true
				}
			case "OperationID":
				if !isEmptyStringLiteral(kv.Value) {
					hasNonEmptyOperationID = true
				}
			}
		}
		if hasEvaluateOnlyTrue && hasNonEmptyOperationID {
			pairs = true
		}
		return true
	})
	return pairs
}

// isHandleInputLitType reports whether t names internal/workflow/engine's
// HandleInput type, either package-qualified (engineAlias.HandleInput, the
// shape every external caller uses — engineAlias resolves the file's actual
// import qualifier, empty if the file does not import the engine package at
// all) or bare (HandleInput, accepted only when the literal's own file is
// package engine itself). A same-named HandleInput type from an unrelated
// package matches neither branch.
func isHandleInputLitType(t ast.Expr, engineAlias string, isEnginePackage bool) bool {
	switch typ := t.(type) {
	case *ast.SelectorExpr:
		ident, ok := typ.X.(*ast.Ident)
		return ok && engineAlias != "" && ident.Name == engineAlias && typ.Sel.Name == "HandleInput"
	case *ast.Ident:
		return isEnginePackage && typ.Name == "HandleInput"
	default:
		return false
	}
}

func isEmptyStringLiteral(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	unquoted, err := strconv.Unquote(lit.Value)
	if err != nil {
		return false
	}
	return unquoted == ""
}
