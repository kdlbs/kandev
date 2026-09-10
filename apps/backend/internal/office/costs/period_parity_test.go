package costs_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// This test implements AC-OFFICE-BUDGET-002.12: it fails the build when the
// set of period values the policy-management surface offers differs from
// the set the write API declares. Both sets are derived by reading their
// respective sources, never from a list maintained alongside them, so
// adding a period value on either side without the other cannot silently
// reproduce the defect AC-OFFICE-BUDGET-002.9 exists to fix.
//
// The declared set comes from models.BudgetPeriod's own Valid() method,
// read via go/ast (same technique as the run-provenance completeness test
// in internal/office/shared): the switch statement's case identifiers are
// resolved against the type's const block, so a value added to the const
// block but never wired into Valid() is correctly excluded from "declared".
//
// The offered set comes from create-budget-form.tsx's period FormField,
// scanned with a regexp scoped to that one block rather than go/ast (the
// file is TypeScript, which go/ast cannot parse).

func thisDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed; cannot locate test file for relative source paths")
	}
	return filepath.Dir(file)
}

// periodConstValues reads path's BudgetPeriod const block and returns a map
// from each declared identifier (e.g. "BudgetPeriodDaily") to its string
// literal value (e.g. "daily"), located by the anchor identifier
// "BudgetPeriodDaily" — never by line number.
func periodConstValues(t *testing.T, path string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		values := constBlockValues(gen)
		if _, ok := values["BudgetPeriodDaily"]; ok {
			return values
		}
	}
	t.Fatalf("BudgetPeriod const block (anchor BudgetPeriodDaily) not found in %s — "+
		"block moved, renamed, or deleted", path)
	return nil
}

func constBlockValues(gen *ast.GenDecl) map[string]string {
	out := map[string]string{}
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range vs.Names {
			if i >= len(vs.Values) {
				continue
			}
			lit, ok := vs.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			val := lit.Value[1 : len(lit.Value)-1] // strip surrounding quotes
			out[name.Name] = val
		}
	}
	return out
}

// declaredPeriodSet derives BudgetPeriod's declared set from Valid()'s own
// switch statement, resolving each case identifier against the const
// block's values rather than assuming every declared constant is valid.
func declaredPeriodSet(t *testing.T) map[string]bool {
	t.Helper()
	dir := thisDir(t)
	path := filepath.Join(dir, "..", "models", "enums.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	constValues := periodConstValues(t, path)

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Valid" || fn.Recv == nil {
			continue
		}
		if !receiverIsBudgetPeriod(fn) {
			continue
		}
		return caseIdentValues(t, fn, constValues)
	}
	t.Fatalf("BudgetPeriod.Valid method not found in %s — method moved, renamed, or deleted", path)
	return nil
}

func receiverIsBudgetPeriod(fn *ast.FuncDecl) bool {
	if len(fn.Recv.List) != 1 {
		return false
	}
	ident, ok := fn.Recv.List[0].Type.(*ast.Ident)
	return ok && ident.Name == "BudgetPeriod"
}

// caseIdentValues walks fn's body for its switch statement and resolves
// every case identifier against constValues, failing the test outright if
// a case expression isn't a plain identifier this test knows how to
// resolve, per AC-OFFICE-BUDGET-002.12's "shall fail on any value present
// in one set and absent from the other" — an unresolved case must not
// silently shrink the declared set.
func caseIdentValues(t *testing.T, fn *ast.FuncDecl, constValues map[string]string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		sw, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		for _, stmt := range sw.Body.List {
			clause, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expr := range clause.List {
				ident, ok := expr.(*ast.Ident)
				if !ok {
					t.Fatalf("BudgetPeriod.Valid case expression %v is not a plain identifier "+
						"this test can resolve", expr)
				}
				val, ok := constValues[ident.Name]
				if !ok {
					t.Fatalf("BudgetPeriod.Valid case identifier %q has no matching const declaration", ident.Name)
				}
				out[val] = true
			}
		}
		found = true
		return false
	})
	if !found {
		t.Fatal("BudgetPeriod.Valid has no switch statement — cannot derive declared period set")
	}
	return out
}

// offeredPeriodSet reads create-budget-form.tsx's period FormField block —
// scoped between the label naming "office:period" and that FormField's
// closing tag, so the scopeType and actionOnExceed Selects' own
// SelectItem values are never counted — and returns every literal
// SelectItem value in it.
func offeredPeriodSet(t *testing.T) map[string]bool {
	t.Helper()
	dir := thisDir(t)
	path := filepath.Join(dir, "..", "..", "..", "..", "web", "app", "office", "workspace", "costs", "create-budget-form.tsx")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	src := string(data)

	labelRe := regexp.MustCompile(`label=\{t\("office:period"\)\}`)
	loc := labelRe.FindStringIndex(src)
	if loc == nil {
		t.Fatalf(`period FormField label={t("office:period")} not found in %s — component restructured`, path)
	}
	rest := src[loc[1]:]
	closeIdx := strings.Index(rest, "</FormField>")
	if closeIdx == -1 {
		t.Fatalf("no closing </FormField> found after the period label in %s", path)
	}
	block := rest[:closeIdx]

	itemRe := regexp.MustCompile(`SelectItem value="([a-z]+)"`)
	matches := itemRe.FindAllStringSubmatch(block, -1)
	if len(matches) == 0 {
		t.Fatalf("no SelectItem value=\"...\" literals found in the period FormField block in %s", path)
	}
	out := map[string]bool{}
	for _, m := range matches {
		out[m[1]] = true
	}
	return out
}

func TestBudgetPeriod_FrontendOfferedSetMatchesDeclaredSet(t *testing.T) {
	declared := declaredPeriodSet(t)
	offered := offeredPeriodSet(t)

	for v := range declared {
		if !offered[v] {
			t.Errorf("period %q is declared by BudgetPeriod.Valid but not offered by "+
				"create-budget-form.tsx's period select", v)
		}
	}
	for v := range offered {
		if !declared[v] {
			t.Errorf("period %q is offered by create-budget-form.tsx's period select but not "+
				"declared by BudgetPeriod.Valid", v)
		}
	}
}
