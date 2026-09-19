package shared_test

// TestEveryRunInsertGoesThroughTheAuthoritativeSeamOrAKnownFallback is
// AC-OFFICE-ENQUEUE-CONSOLIDATION-001.3's mandated structural test: rather
// than trusting that every future insert into the runs table was
// remembered to route through runs/service.Service.QueueRun by hand, this
// parses every non-test .go file under internal/office and internal/runs
// for a CreateRun(...)/CreateRunTx(...) method call, and fails when one
// appears anywhere other than the small, explicitly reviewed allowlist
// below.
//
// internal/office/wakeup/dispatcher.go's createFreshRun (Review round 1
// finding 1) is the worked example this guard exists because of: it built
// a models.Run{} literal and called repo.CreateRun directly, bypassing
// every causation/priority/workspace resolution the seam performs. That
// insert call is gone (it now routes through office/service.Service's
// QueueRunFromWakeup, itself allowlisted below at the seam it delegates
// to), so this test also pins that the fix stays fixed.
//
// Call sites named CreateRun/CreateRunTx outside internal/office and
// internal/runs belong to unrelated domains (internal/automation,
// internal/review, internal/system/storage each declare their own
// same-named method on an unrelated type) and are out of this test's
// scope by construction — it never walks those directories.
import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// runInsertAllowlist names the only (file, function) pairs permitted to
// call CreateRun/CreateRunTx on a runs-table-backed repository, file paths
// relative to the apps/backend module root. This is function-scoped, not
// file-scoped: a file appearing below is not exempted wholesale, so a new
// direct-insert function added later to an already-allowlisted file still
// fails this test unless it is reviewed and added by name.
//
//   - internal/runs/repository/sqlite/runs.go: CreateRun/CreateRunTx's own
//     definitions. This is the implementation, not a bypass of it.
//   - internal/runs/service/service.go: insertRun, the authoritative
//     seam every other caller in the tree is required to route through.
//   - internal/office/service/run.go: queueRunInline, reached only when
//     no runs service is wired (SetRunsService never called — untested
//     configurations and a handful of tests that don't exercise
//     causation, never production). AC-OFFICE-ENQUEUE-CONSOLIDATION-001.6's
//     full removal is tracked separately; not attempted this round (see
//     the task plan's "Investigated: enqueue consolidation" note).
//     internal/office/scheduler/run.go's equivalent legacy fallback was
//     removed entirely (AC-OFFICE-ENQUEUE-CONSOLIDATION-001.6), so it no
//     longer needs an entry here.
//   - internal/office/testharness/routes_office.go: shared test-support
//     scaffolding (package testharness, imported only by _test.go files
//     across the office tree) — not production code, despite lacking the
//     _test.go filename suffix this walk otherwise excludes by.
var runInsertAllowlist = map[string]map[string]bool{
	"internal/runs/repository/sqlite/runs.go": {
		"CreateRunTx": true,
		"CreateRun":   true,
	},
	"internal/runs/service/service.go": {
		"insertRun": true,
	},
	"internal/office/service/run.go": {
		"queueRunInline": true,
	},
	"internal/office/testharness/routes_office.go": {
		"seedAgentFailureHandler": true,
		"createSeededRun":         true,
	},
}

func TestEveryRunInsertGoesThroughTheAuthoritativeSeamOrAKnownFallback(t *testing.T) {
	backendRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolve apps/backend root: %v", err)
	}

	var violations []string
	fset := token.NewFileSet()
	for _, dir := range []string{"internal/office", "internal/runs"} {
		root := filepath.Join(backendRoot, dir)
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
			rel, relErr := filepath.Rel(backendRoot, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			violations = append(violations, runInsertViolationsInFile(rel, file, runInsertAllowlist[rel])...)
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", dir, walkErr)
		}
	}

	if len(violations) > 0 {
		t.Errorf("found CreateRun/CreateRunTx call(s) outside the reviewed allowlist:\n%s\n"+
			"AC-OFFICE-ENQUEUE-CONSOLIDATION-001.3 requires every runs-table insert to go through "+
			"runs/service.Service.QueueRun (directly, or via office/service.Service's delegating "+
			"helpers). Route the new call through the seam, or add it to runInsertAllowlist in "+
			"internal/office/shared/enqueue_consolidation_test.go with a reviewed justification.",
			strings.Join(violations, "\n"))
	}
}

// runInsertViolationsInFile returns one "path: func Name" entry for every
// top-level function declaration in file whose body calls CreateRun or
// CreateRunTx, unless that function's name is in allowed. Checking per
// function (rather than once for the whole file) is what makes the
// allowlist function-scoped: an already-allowlisted file gets no blanket
// pass, so a new direct-insert function added there later still fails
// this test unless it is reviewed and named explicitly.
func runInsertViolationsInFile(path string, file *ast.File, allowed map[string]bool) []string {
	var violations []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !callsRunInsert(fn.Body) {
			continue
		}
		if allowed[fn.Name.Name] {
			continue
		}
		violations = append(violations, fmt.Sprintf("%s: func %s", path, fn.Name.Name))
	}
	return violations
}

// callsRunInsert reports whether body contains any call expression whose
// selector method name is exactly CreateRun or CreateRunTx.
func callsRunInsert(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == "CreateRun" || sel.Sel.Name == "CreateRunTx" {
			found = true
			return false
		}
		return true
	})
	return found
}

// TestRunInsertViolationsInFile_ScopesToFunctionNotWholeFile is the
// regression this file's allowlist switched from map[string]bool to
// map[string]map[string]bool to guard against: an already-allowlisted
// file no longer gets a blanket pass, so a second, unreviewed function in
// the same file that also calls CreateRun must still be reported.
func TestRunInsertViolationsInFile_ScopesToFunctionNotWholeFile(t *testing.T) {
	src := `package example

func allowedCaller(r repo) {
	r.CreateRun(nil)
}

func unreviewedCaller(r repo) {
	r.CreateRunTx(nil, nil)
}

func noCallAtAll() {}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "example.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	violations := runInsertViolationsInFile("example.go", file, map[string]bool{"allowedCaller": true})
	if len(violations) != 1 {
		t.Fatalf("violations = %v, want exactly 1 (unreviewedCaller)", violations)
	}
	if violations[0] != "example.go: func unreviewedCaller" {
		t.Fatalf("violation = %q, want %q", violations[0], "example.go: func unreviewedCaller")
	}
}
