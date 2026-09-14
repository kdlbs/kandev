package shared_test

// TestWakeReasonRegistry_CoversEveryDeclaredRunReason is
// AC-OFFICE-BACKPRESSURE-001.4/.8's mandated exhaustiveness test: rather
// than trusting that every RunReasonXxx constant declared anywhere under
// internal/office was remembered to be added to shared.WakeReasonRegistry
// by hand, this parses every non-test .go file under internal/office for
// a const declaration named RunReasonXxx (or the package-private
// legacyRunReasonXxx spelling office/service/run.go uses) whose value is a
// raw string literal, and fails when that literal is missing from the
// registry.
//
// office/scheduler/run.go's reactivity-pipeline reasons
// (RunReasonTaskUnblocked, RunReasonTaskReopened, ...) are the worked
// example this guard exists because of: they were declared and reachable
// in production (scheduler/reactivity.go) for some time without ever
// being added to WakeReasonRegistry, silently falling through to the
// registry-miss "event" default instead of resolving through an explicit
// rule.
//
// A constant assigned from another named identifier rather than a raw
// string literal (e.g. RunReasonHeartbeat = shared.RunReasonHeartbeat) is
// not itself a new reason value — it aliases one declared elsewhere in the
// tree, which this same walk finds and checks under its own name.
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

	"github.com/kandev/kandev/internal/office/shared"
)

func TestWakeReasonRegistry_CoversEveryDeclaredRunReason(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve internal/office root: %v", err)
	}

	var missing []string
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
		for _, reason := range declaredRunReasonLiterals(file) {
			if _, ok := shared.PriorityClassForReason(reason); !ok {
				pos := fset.Position(file.Pos())
				missing = append(missing, fmt.Sprintf("%q (declared in %s)", reason, pos.Filename))
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk internal/office: %v", walkErr)
	}

	if len(missing) > 0 {
		t.Errorf("shared.WakeReasonRegistry is missing an entry for:\n%s\n"+
			"AC-OFFICE-BACKPRESSURE-001.4/.8 requires every declared wake reason to "+
			"resolve through an explicit registry rule. Add it to WakeReasonRegistry "+
			"in internal/office/shared/wakereasons.go.",
			strings.Join(missing, "\n"))
	}
}

// declaredRunReasonLiterals returns the string literal value of every
// top-level const in file whose identifier starts with "RunReason" or
// "legacyRunReason" and whose value is itself a raw string literal (not
// an alias of another identifier).
func declaredRunReasonLiterals(file *ast.File) []string {
	var reasons []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vspec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vspec.Names {
				if !isRunReasonIdent(name.Name) || i >= len(vspec.Values) {
					continue
				}
				lit, ok := vspec.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				reasons = append(reasons, value)
			}
		}
	}
	return reasons
}

func isRunReasonIdent(name string) bool {
	return strings.HasPrefix(name, "RunReason") || strings.HasPrefix(name, "legacyRunReason")
}
