package sqlite

// TestStepTransitionWritersArePinned is the writer-health control that
// matters most, per the spec: the realistic failure mode is not the ledger
// writer breaking, it's a NEW production statement mutating
// tasks.workflow_step_id that nobody wired into it. This walks the package's
// own source (go/parser, not a line regex — every statement here is a
// multi-line raw string literal, which a regex over raw lines misreports on)
// and asserts the exact set of functions containing such a statement matches
// the registered, ledger-wired set. Add a new mutating statement and this
// test fails until it is registered below AND wired to recordStepTransition.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// registeredStepMutators is the closed set of functions in this package
// known to mutate tasks.workflow_step_id, each already wired to
// recordStepTransition, keyed by "ReceiverType.FuncName" (see funcIdentity)
// rather than a bare function name — a bare name collides across distinct
// receiver types with the same method name (see
// TestFindWorkflowStepIDMutatorsCatchesEvasiveShapes shape (c)). A new
// function containing a matching statement that is not in this set means
// either: (a) it is a genuine new step-mutation path — wire it to
// recordStepTransition and add it here; or (b) it is a false positive (e.g.
// a table-rebuild migration copying existing rows verbatim) — narrow the
// detection below rather than registering it.
var registeredStepMutators = []string{
	"Repository.insertTaskTx",
	"Repository.updateTaskTx",
	"Repository.UpdateTaskIfWorkflowStepHasCapacity",
	"Repository.PromoteQueuedTaskIfWorkflowStepHasCapacity",
	"Repository.RestoreTaskMessageRollbackIfSessionState",
	"Repository.AddTaskToWorkflow",
	"Repository.RemoveTaskFromWorkflow",
}

func TestStepTransitionWritersArePinned(t *testing.T) {
	found, err := findWorkflowStepIDMutators(t, ".")
	if err != nil {
		t.Fatalf("scan package for workflow_step_id mutators: %v", err)
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

// findWorkflowStepIDMutators recursively parses every non-test .go file
// under dir (plus .go.txt, so isolated test fixtures under testdata/ can be
// parsed without ever compiling as part of any real package) and returns the
// set of function identities ("ReceiverType.FuncName", or a bare name for a
// receiver-less function) whose body mutates tasks.workflow_step_id — either
// directly via a string literal, indirectly via a package-level const/var
// the body references by name, or via a call to a local fmt.Sprintf-based
// column setter passed the literal column name "workflow_step_id". A
// directory literally named "testdata" is skipped while walking so
// production scans (dir=".") never pick up fixtures.
func findWorkflowStepIDMutators(t *testing.T, dir string) (map[string]bool, error) {
	t.Helper()

	paths, err := collectSourceFiles(dir)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(paths))
	for _, path := range paths {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	constants := collectPackageStringConstants(files)
	formatSetters := collectFormatColumnSetters(files, constants)

	found := make(map[string]bool)
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			if functionMutatesWorkflowStepID(fn.Body, constants, formatSetters) {
				found[funcIdentity(fn)] = true
			}
			return true
		})
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

// funcIdentity qualifies fn's name with its receiver type ("Repository.
// updateTaskTx"), or returns the bare name for a receiver-less function, so
// two distinct types' same-named methods are never collapsed into one key.
func funcIdentity(fn *ast.FuncDecl) string {
	recv := receiverTypeName(fn)
	if recv == "" {
		return fn.Name.Name
	}
	return recv + "." + fn.Name.Name
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
// and a writer placed in a subdirectory. testdata/ is otherwise skipped by
// findWorkflowStepIDMutators so these fixtures never affect
// TestStepTransitionWritersArePinned's scan of the real package.
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
		"NestedRepo.deeplyNestedStepMutator",         // (d) nested subdirectory
	}
	for _, name := range want {
		if !found[name] {
			t.Errorf("expected findWorkflowStepIDMutators to catch %q, but it did not (found: %v)", name, found)
		}
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
