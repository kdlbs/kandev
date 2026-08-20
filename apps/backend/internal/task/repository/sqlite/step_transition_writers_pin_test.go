package sqlite

// TestStepTransitionWritersArePinned is the writer-health control that
// matters most, per the spec: the realistic failure mode is not the ledger
// writer breaking, it's a NEW production statement mutating
// tasks.workflow_step_id that nobody wired into it. This walks the entire
// backend source tree (go/parser, not a line regex — every statement here is
// a multi-line raw string literal, which a regex over raw lines misreports
// on), rooted at apps/backend/internal (see findBackendSourceRoot) rather
// than this package's own directory — a writer added in ANY backend
// package, not only this one, must be able to fail this test — and asserts
// the exact set of functions containing such a statement matches the
// registered, ledger-wired set. Add a new mutating statement and this test
// fails until it is registered below AND wired to recordStepTransition.

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

// registeredStepMutators is the closed set of functions in this package
// known to mutate tasks.workflow_step_id, each already wired to
// recordStepTransition, keyed by "packageRelDir/ReceiverType.FuncName" (see
// funcIdentity) rather than a bare "ReceiverType.FuncName" — a bare
// receiver-qualified name still collides across distinct PACKAGES declaring
// the same receiver type (this codebase declares a type literally named
// Repository in 11 different backend packages; see Review round 2 Finding A
// / TestFindWorkflowStepIDMutatorsQualifiesByPackage), and a fully bare name
// collides across distinct receiver types with the same method name in one
// package (see TestFindWorkflowStepIDMutatorsCatchesEvasiveShapes shape
// (c)). packageRelDir is this package's path relative to the scan root
// (findBackendSourceRoot's apps/backend/internal boundary) — always
// "task/repository/sqlite" for entries in this file, since every writer
// registered here lives in this package. A new function containing a
// matching statement that is not in this set means either: (a) it is a
// genuine new step-mutation path — wire it to recordStepTransition and add
// it here; or (b) it is a false positive (e.g. a table-rebuild migration
// copying existing rows verbatim) — narrow the detection below rather than
// registering it.
var registeredStepMutators = []string{
	"task/repository/sqlite/Repository.insertTaskTx",
	"task/repository/sqlite/Repository.updateTaskTx",
	"task/repository/sqlite/Repository.UpdateTaskIfWorkflowStepHasCapacity",
	"task/repository/sqlite/Repository.PromoteQueuedTaskIfWorkflowStepHasCapacity",
	"task/repository/sqlite/Repository.RestoreTaskMessageRollbackIfSessionState",
	"task/repository/sqlite/Repository.AddTaskToWorkflow",
	"task/repository/sqlite/Repository.RemoveTaskFromWorkflow",
}

func TestStepTransitionWritersArePinned(t *testing.T) {
	root, err := findBackendSourceRoot(".")
	if err != nil {
		t.Fatalf("locate backend source root: %v", err)
	}
	// Sanity check that the scan root is genuinely the wide boundary and not
	// silently regressed back to "." (this package's own directory) — a
	// writer added in ANY backend package, not just this one, must be able
	// to fail this test. internal/office is an arbitrary sibling package
	// known to exist; its presence under root proves the walk spans more
	// than internal/task/repository/sqlite.
	if filepath.Base(root) != "internal" {
		t.Fatalf("scan root %q is not the backend internal/ directory", root)
	}
	if _, statErr := os.Stat(filepath.Join(root, "office")); statErr != nil {
		t.Fatalf("scan root %q does not contain sibling package internal/office — root is narrower than the whole backend source tree: %v", root, statErr)
	}

	found, err := findWorkflowStepIDMutators(t, root)
	if err != nil {
		t.Fatalf("scan backend source for workflow_step_id mutators: %v", err)
	}

	registered := make(map[string]bool, len(registeredStepMutators))
	for _, name := range registeredStepMutators {
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
			"function(s) %v contain a statement that mutates tasks.workflow_step_id "+
				"but are not registered in registeredStepMutators. If this is a genuine "+
				"new step-mutation path: wire it to (*Repository).recordStepTransition "+
				"inside its own transaction (see step_transitions.go), then add its name "+
				"to registeredStepMutators in this file. If this is a false positive "+
				"(e.g. a migration copying rows verbatim), narrow the detection in "+
				"findWorkflowStepIDMutators instead of registering it.",
			unregistered)
	}

	var missing []string
	for _, name := range registeredStepMutators {
		if !found[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf(
			"registeredStepMutators lists %v but no matching statement was found in "+
				"this package — the function was likely renamed, removed, or its SQL "+
				"literal changed shape. Update registeredStepMutators to match reality.",
			missing)
	}
}

// findBackendSourceRoot walks up from startDir looking for the Go module
// root (the directory containing go.mod) and returns its internal/
// subdirectory. This is the production scan's boundary: internal/ is the
// entire body of backend source that could define a tasks.workflow_step_id
// writer, so rooting the pinning test there instead of "." (this package's
// own directory) is what actually closes false-negative shape (d) — a
// writer added in any OTHER backend package, not just a nested subdirectory
// of this one. go test always runs with the package directory as its
// working directory, so startDir="." from this package resolves reliably to
// apps/backend/internal regardless of where the repo checkout lives.
func findBackendSourceRoot(startDir string) (string, error) {
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

// findWorkflowStepIDMutators recursively parses every non-test .go file
// under dir (plus .go.txt, so isolated test fixtures under testdata/ can be
// parsed without ever compiling as part of any real package) and returns the
// set of function identities ("ReceiverType.FuncName", or a bare name for a
// receiver-less function) whose body mutates tasks.workflow_step_id — either
// directly via a string literal, indirectly via a package-level const/var
// the body references by name, or via a call to a local fmt.Sprintf-based
// column setter passed the literal column name "workflow_step_id". A
// directory literally named "testdata" is skipped while walking so
// production scans never pick up fixtures.
//
// dir can span many packages (the production scan roots at the whole
// backend internal/ tree — see findBackendSourceRoot). Constants and
// format-setters are therefore resolved per package, grouping parsed files
// by their containing directory and running the const/format-setter/mutation
// pipeline independently per directory, rather than flattening every parsed
// file's package-level consts into one repo-wide map. Two unrelated packages
// can legally declare a const or helper function with the same bare name
// (e.g. two packages each with their own `updateQuery` constant); a flat map
// would let one package's SQL literal leak into another's mutation check —
// or mask a genuine literal — purely by name collision. Each directory is
// exactly one Go package once _test.go files are excluded (which this scan
// already does), so scoping resolution to the files parsed from one
// directory at a time is correct without tracking real package identifiers.
func findWorkflowStepIDMutators(t *testing.T, dir string) (map[string]bool, error) {
	t.Helper()

	paths, err := collectSourceFiles(dir)
	if err != nil {
		return nil, err
	}

	byDir := make(map[string][]string)
	for _, path := range paths {
		d := filepath.Dir(path)
		byDir[d] = append(byDir[d], path)
	}

	found := make(map[string]bool)
	fset := token.NewFileSet()
	for groupDir, dirPaths := range byDir {
		relDir, relErr := filepath.Rel(dir, groupDir)
		if relErr != nil {
			return nil, fmt.Errorf("resolve package directory %q relative to scan root %q: %w", groupDir, dir, relErr)
		}

		files := make([]*ast.File, 0, len(dirPaths))
		for _, path := range dirPaths {
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil, err
			}
			files = append(files, file)
		}

		constants := collectPackageStringConstants(files)
		formatSetters := collectFormatColumnSetters(files, constants)

		for _, file := range files {
			ast.Inspect(file, func(n ast.Node) bool {
				fn, ok := n.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					return true
				}
				if functionMutatesWorkflowStepID(fn.Body, constants, formatSetters) {
					found[funcIdentity(fn, relDir)] = true
				}
				return true
			})
		}
	}
	return found, nil
}

// collectSourceFiles walks dir recursively, collecting non-test .go and
// .go.txt file paths. Directories named "testdata" are skipped so a
// production scan rooted above testdata/ never parses fixture files as
// real package source.
func collectSourceFiles(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == "testdata" && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		if strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".go.txt") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// collectPackageStringConstants maps every package-level const/var string
// declaration's name to its literal value, across all parsed files. Only
// top-level declarations are visited (file.Decls), so a function-local
// const or var is never captured here.
func collectPackageStringConstants(files []*ast.File) map[string]string {
	consts := make(map[string]string)
	for _, file := range files {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || (genDecl.Tok != token.CONST && genDecl.Tok != token.VAR) {
				continue
			}
			collectValueSpecStrings(genDecl, consts)
		}
	}
	return consts
}

func collectValueSpecStrings(genDecl *ast.GenDecl, out map[string]string) {
	for _, spec := range genDecl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range valueSpec.Names {
			if i >= len(valueSpec.Values) {
				continue
			}
			lit, ok := valueSpec.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			out[name.Name] = strings.Trim(lit.Value, "`\"")
		}
	}
}

// collectFormatColumnSetters returns the bare names of functions whose body
// calls fmt.Sprintf (or any *.Sprintf selector call) with a format string
// shaped like "UPDATE tasks SET %s ..." — the taskScalarColumns/
// execTaskScalar pattern where the mutated column is a parameter, not a
// literal in this function's own body. Keyed by bare name because call
// sites are matched by bare callee name below (no full type resolution).
func collectFormatColumnSetters(files []*ast.File, constants map[string]string) map[string]bool {
	setters := make(map[string]bool)
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			if functionHasScalarSetterFormat(fn.Body, constants) {
				setters[fn.Name.Name] = true
			}
			return true
		})
	}
	return setters
}

func functionHasScalarSetterFormat(body *ast.BlockStmt, constants map[string]string) bool {
	has := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Sprintf" {
			return true
		}
		if fmtStr, ok := resolveStringLiteral(call.Args[0], constants); ok && isScalarSetterFormat(fmtStr) {
			has = true
		}
		return true
	})
	return has
}

func isScalarSetterFormat(s string) bool {
	return strings.Contains(s, "UPDATE tasks") && strings.Contains(s, "SET %s")
}

// resolveStringLiteral resolves expr to a string value, either directly
// (a string literal) or indirectly (an identifier naming a package-level
// const/var string collected in constants).
func resolveStringLiteral(expr ast.Expr, constants map[string]string) (string, bool) {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return strings.Trim(v.Value, "`\""), true
		}
	case *ast.Ident:
		if val, ok := constants[v.Name]; ok {
			return val, true
		}
	}
	return "", false
}

// functionMutatesWorkflowStepID reports whether body mutates
// tasks.workflow_step_id via any of: a direct string literal, an identifier
// referencing a package-level const/var string, or a call to a known
// fmt.Sprintf-based column setter passed the literal column name
// "workflow_step_id".
func functionMutatesWorkflowStepID(body *ast.BlockStmt, constants map[string]string, formatSetters map[string]bool) bool {
	mutates := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.BasicLit:
			if v.Kind == token.STRING && literalMutatesWorkflowStepID(strings.Trim(v.Value, "`\"")) {
				mutates = true
			}
		case *ast.Ident:
			if val, ok := constants[v.Name]; ok && literalMutatesWorkflowStepID(val) {
				mutates = true
			}
		case *ast.CallExpr:
			if callTargetsColumnSetter(v, formatSetters) {
				mutates = true
			}
		}
		return true
	})
	return mutates
}

// callTargetsColumnSetter reports whether call invokes a known
// fmt.Sprintf-based column setter (by bare callee name) with the literal
// string "workflow_step_id" as one of its arguments.
func callTargetsColumnSetter(call *ast.CallExpr, formatSetters map[string]bool) bool {
	name := calleeName(call.Fun)
	if name == "" || !formatSetters[name] {
		return false
	}
	for _, arg := range call.Args {
		lit, ok := arg.(*ast.BasicLit)
		if ok && lit.Kind == token.STRING && strings.Trim(lit.Value, "`\"") == "workflow_step_id" {
			return true
		}
	}
	return false
}

func calleeName(fun ast.Expr) string {
	switch v := fun.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return v.Sel.Name
	default:
		return ""
	}
}

// funcIdentity qualifies fn's name with its receiver type and its
// containing package's directory relative to the scan root
// ("task/repository/sqlite/Repository.updateTaskTx"), or "relDir/FuncName"
// for a receiver-less function, so two distinct types' same-named methods
// are never collapsed into one key (within a package) AND two distinct
// packages declaring the same receiver type name are never collapsed into
// one key (across packages — see Review round 2 Finding A: this codebase
// declares a type literally named Repository in 11 different backend
// packages, so "Repository.updateTaskTx" alone is not a safe key once the
// scan spans more than one package). relDir is "." for a file directly in
// the scanned root, which yields an unprefixed identity — used by the
// evasive-shapes fixtures that live directly under testdata/pinfixtures.
func funcIdentity(fn *ast.FuncDecl, relDir string) string {
	name := fn.Name.Name
	if recv := receiverTypeName(fn); recv != "" {
		name = recv + "." + name
	}
	if relDir == "" || relDir == "." {
		return name
	}
	return filepath.ToSlash(relDir) + "/" + name
}

func receiverTypeName(fn *ast.FuncDecl) string {
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

// TestFindWorkflowStepIDMutatorsCatchesEvasiveShapes runs the detector
// against permanent fixtures under testdata/pinfixtures/, one per
// false-negative shape identified in the mutation review: a const-hoisted
// SQL literal, an fmt.Sprintf-interpolated column name (the exact shape
// already live in internal/office/repository/sqlite/tasks.go's
// execTaskScalar), two distinct receiver types sharing a bare method name,
// and a writer placed in a subdirectory nested under whatever root the
// detector is given (proving recursion, not scan-root width — see
// TestFindWorkflowStepIDMutatorsCatchesSiblingPackageWriter below for that).
// testdata/ is otherwise skipped by findWorkflowStepIDMutators so these
// fixtures never affect TestStepTransitionWritersArePinned's scan of real
// backend source.
func TestFindWorkflowStepIDMutatorsCatchesEvasiveShapes(t *testing.T) {
	found, err := findWorkflowStepIDMutators(t, filepath.Join("testdata", "pinfixtures"))
	if err != nil {
		t.Fatalf("scan fixtures: %v", err)
	}

	want := []string{
		"FixtureRepo.constHoistedStepMutator",        // (a) const-hoisted SQL literal
		"FixtureRepo.sprintfInterpolatedStepMutator", // (b) fmt.Sprintf column interpolation
		"RepoA.updateTaskTx",                         // (c) receiver-qualified identity
		"RepoB.updateTaskTx",                         // (c) distinct from RepoA's, not collapsed
		"nested/NestedRepo.deeplyNestedStepMutator",  // (d, partial) recursion into a subdirectory — package-qualified since it's one level below the scan root
	}
	if len(found) != len(want) {
		t.Errorf("findWorkflowStepIDMutators found %d mutators from fixtures, want %d: found=%v", len(found), len(want), found)
	}
	for _, name := range want {
		if !found[name] {
			t.Errorf("expected findWorkflowStepIDMutators to catch %q, but it did not (found: %v)", name, found)
		}
	}
}

// TestFindWorkflowStepIDMutatorsCatchesSiblingPackageWriter is the permanent
// regression proof for the rest of false-negative shape (d): "single-package
// scope misses writers added outside the sqlite repo package." The nested
// fixture above proves the detector recurses into subdirectories of
// whatever root it is given; it does not prove the production scan's root
// actually spans sibling packages, since before this fix the root was "."
// (this package's own directory) and recursion had nothing above it to
// explore. This test targets that boundary directly, independent of the
// real repo layout: it builds a synthetic two-package tree in a t.TempDir()
// — one package with a genuine tasks.workflow_step_id writer, a sibling
// package with none — and asserts findWorkflowStepIDMutators, given the
// tree's root, finds the writer inside the sibling package. That is exactly
// the guarantee TestStepTransitionWritersArePinned now depends on when
// rooted at findBackendSourceRoot's apps/backend/internal boundary: a
// writer in ANY sibling package under that root must be caught, not only
// ones nested under this package's own directory.
func TestFindWorkflowStepIDMutatorsCatchesSiblingPackageWriter(t *testing.T) {
	root := t.TempDir()
	pkgWithWriter := filepath.Join(root, "pkgwithwriter")
	pkgWithoutWriter := filepath.Join(root, "pkgwithoutwriter")
	if err := os.MkdirAll(pkgWithWriter, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", pkgWithWriter, err)
	}
	if err := os.MkdirAll(pkgWithoutWriter, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", pkgWithoutWriter, err)
	}
	writePinTestFixtureFile(t, filepath.Join(pkgWithWriter, "writer.go"), `package pkgwithwriter

type Repository struct{}

func (r *Repository) mutateStep() string {
	return "UPDATE tasks SET workflow_step_id = ? WHERE id = ?"
}
`)
	writePinTestFixtureFile(t, filepath.Join(pkgWithoutWriter, "other.go"), `package pkgwithoutwriter

func Noop() {}
`)

	found, err := findWorkflowStepIDMutators(t, root)
	if err != nil {
		t.Fatalf("scan synthetic sibling-package tree: %v", err)
	}
	if !found["pkgwithwriter/Repository.mutateStep"] {
		t.Fatalf("expected a writer in a sibling package under %s to be found, package-qualified; found=%v", root, found)
	}
}

// TestFindWorkflowStepIDMutatorsQualifiesByPackage is the permanent
// regression proof for Review round 2 Finding A: funcIdentity used to key
// solely on "ReceiverType.FuncName" with no package component, so two
// different backend packages declaring the same receiver type name — this
// codebase declares a type literally named Repository in 11 different
// packages — and the same method name collided into a single identity. A
// genuine writer in one package could then silently "borrow" another
// package's already-registered status: TestStepTransitionWritersArePinned
// would stay green even though the second writer was never wired to
// recordStepTransition. This builds a synthetic two-package tree where both
// packages declare `type Repository struct{}` with an identical method name,
// each mutating tasks.workflow_step_id via its own distinct SQL text, and
// asserts the detector reports two distinct package-qualified identities,
// not one collapsed entry.
func TestFindWorkflowStepIDMutatorsQualifiesByPackage(t *testing.T) {
	root := t.TempDir()
	pkgA := filepath.Join(root, "pkga")
	pkgB := filepath.Join(root, "pkgb")
	if err := os.MkdirAll(pkgA, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", pkgA, err)
	}
	if err := os.MkdirAll(pkgB, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", pkgB, err)
	}
	writePinTestFixtureFile(t, filepath.Join(pkgA, "repo.go"), `package pkga

type Repository struct{}

func (r *Repository) updateTaskTx() string {
	return "UPDATE tasks SET workflow_step_id = ?, updated_at = ? WHERE id = ?"
}
`)
	writePinTestFixtureFile(t, filepath.Join(pkgB, "repo.go"), `package pkgb

type Repository struct{}

func (r *Repository) updateTaskTx() string {
	return "UPDATE tasks SET workflow_step_id = ? WHERE id = ?"
}
`)

	found, err := findWorkflowStepIDMutators(t, root)
	if err != nil {
		t.Fatalf("scan synthetic colliding-identity tree: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("expected 2 distinct package-qualified identities from two colliding Repository.updateTaskTx pairs, got %d: %v", len(found), found)
	}
	if !found["pkga/Repository.updateTaskTx"] {
		t.Errorf("expected pkga's writer to be found under its own package-qualified identity; found=%v", found)
	}
	if !found["pkgb/Repository.updateTaskTx"] {
		t.Errorf("expected pkgb's writer to be found under its own package-qualified identity; found=%v", found)
	}
}

func writePinTestFixtureFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func literalMutatesWorkflowStepID(sql string) bool {
	insertsWithColumn := strings.Contains(sql, "INSERT INTO tasks (") && strings.Contains(sql, "workflow_step_id")
	// "workflow_step_id = " (not just "= ?") also catches an UPDATE that
	// assigns a literal value directly, e.g. RemoveTaskFromWorkflow's
	// "workflow_step_id = ''" — a parameter-only pattern would miss it.
	updatesColumn := strings.Contains(sql, "UPDATE tasks") && strings.Contains(sql, "workflow_step_id = ")
	return insertsWithColumn || updatesColumn
}
