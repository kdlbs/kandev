package backendapp

// TestEveryQueueRunRequestLiteralDeclaresActorKind is AC-OFFICE-RUN-CAUSATION-001.23's
// "system shall have a test that fails when an enqueue path can reach the
// queue without a source": rather than a hand-maintained list of the paths
// known to pass an actor today (the AC calls that insufficient — see
// AC-OFFICE-BACKPRESSURE-001.8's reasoning, "a list maintained by hand is
// what fell behind"), this parses every non-test .go file under internal/
// and fails on any runsservice.QueueRunRequest{} composite literal that
// does not explicitly key ActorKind. A new enqueue path added anywhere in
// the tree without sourcing an actor fails this test on the next run,
// rather than silently reaching the queue and falling back to
// office_launch_actor_missing_total.
//
// runsServiceEngineAdapter.QueueRun (queue_run_actor_source_test.go's
// sibling, runs_engine_adapter_actor_test.go) is the worked example this
// guard exists because of: it reached runs/service.QueueRun with no
// ActorKind at all until it was taught to read the task-boundary carrier.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// queueRunRequestImportPath is the package whose QueueRunRequest this test
// guards. internal/workflow/engine declares its own same-named type; that
// one is out of scope here because it never reaches the queue directly —
// every value flows through an adapter that builds a
// runsservice.QueueRunRequest, which this test does check.
const queueRunRequestImportPath = "github.com/kandev/kandev/internal/runs/service"

func TestEveryQueueRunRequestLiteralDeclaresActorKind(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve internal/ root: %v", err)
	}

	var violations []string
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		alias := runsServiceImportAlias(file)
		if alias == "" {
			return nil
		}
		violations = append(violations, findActorlessQueueRunRequests(fset, file, alias)...)
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk internal/: %v", walkErr)
	}

	if len(violations) > 0 {
		t.Errorf("runsservice.QueueRunRequest{} built without an explicit ActorKind key at:\n%s\n"+
			"AC-OFFICE-RUN-CAUSATION-001.23 requires every enqueue path to declare a named actor "+
			"source (a runtime action's agent profile, the authenticated user, system for a routine "+
			"fire, or the task-boundary carrier for a task-caused wake). Set ActorKind explicitly, "+
			"or route through a helper that already does.",
			strings.Join(violations, "\n"))
	}
}

// runsServiceImportAlias returns the local identifier file uses for
// queueRunRequestImportPath, or "" if the file does not import it. A blank
// or dot import returns "" too: neither produces a selector literal this
// test's AST match can key off, and no import site in the tree uses either
// form for this package today.
func runsServiceImportAlias(file *ast.File) string {
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != queueRunRequestImportPath {
			continue
		}
		if imp.Name == nil {
			return "service"
		}
		if imp.Name.Name == "_" || imp.Name.Name == "." {
			return ""
		}
		return imp.Name.Name
	}
	return ""
}

// findActorlessQueueRunRequests returns "file:line" for every
// alias.QueueRunRequest{} composite literal in file lacking an ActorKind
// key.
func findActorlessQueueRunRequests(fset *token.FileSet, file *ast.File, alias string) []string {
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || pkgIdent.Name != alias || sel.Sel.Name != "QueueRunRequest" {
			return true
		}
		if !declaresActorKind(lit) {
			pos := fset.Position(lit.Pos())
			found = append(found, fmt.Sprintf("%s:%d", pos.Filename, pos.Line))
		}
		return true
	})
	return found
}

// declaresActorKind reports whether a QueueRunRequest composite literal
// keys ActorKind. Every literal in the tree today uses keyed fields
// (struct this wide with a positional literal fails vet's composite
// literal check on its own), so an unkeyed literal is not a case this
// walks needs to handle.
func declaresActorKind(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == "ActorKind" {
			return true
		}
	}
	return false
}
