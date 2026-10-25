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
// tree, which this same walk finds and checks under its own name. Since
// every consumer now aliases shared's canonical declarations (see
// TestWakeReasonRegistry_DeclaresEveryReasonOnlyInSharedPackage below),
// this walk only ever finds raw literals inside internal/office/shared
// itself in practice — it stays here, unchanged, as the exhaustiveness
// check on that single declaration site.
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

// runReasonLiteral is one RunReasonXxx/legacyRunReasonXxx const whose
// value is a raw string literal, with the file it was declared in.
type runReasonLiteral struct {
	name  string
	value string
	file  string
}

func TestWakeReasonRegistry_CoversEveryDeclaredRunReason(t *testing.T) {
	literals, walkErr := walkRunReasonLiterals(t)
	if walkErr != nil {
		t.Fatalf("walk internal/office: %v", walkErr)
	}

	var missing []string
	for _, lit := range literals {
		if _, ok := shared.PriorityClassForReason(lit.value); !ok {
			missing = append(missing, fmt.Sprintf("%q (declared in %s)", lit.value, lit.file))
		}
	}
	if len(missing) > 0 {
		t.Errorf("shared.WakeReasonRegistry is missing an entry for:\n%s\n"+
			"AC-OFFICE-BACKPRESSURE-001.4/.8 requires every declared wake reason to "+
			"resolve through an explicit registry rule. Add it to WakeReasonRegistry "+
			"in internal/office/shared/wakereasons.go.",
			strings.Join(missing, "\n"))
	}
}

// TestWakeReasonRegistry_DeclaresEveryReasonOnlyInSharedPackage is
// AC-OFFICE-BACKPRESSURE-001.8's other half: "the registry shall be the
// single place a wake reason is declared, and the system shall have a
// test that fails when a wake-reason constant is declared outside it."
// TestWakeReasonRegistry_CoversEveryDeclaredRunReason only checks that a
// declared value resolves somewhere in the registry — it cannot fail when
// the SAME value is independently re-declared as a second raw literal
// outside internal/office/shared, because that second declaration
// resolves through the registry just fine — office/service/run.go and
// office/scheduler/run.go independently declaring their own raw-literal
// copy of the same reason (task_assigned, task_comment, ...) with no
// alias tying the pair together would silently create two different wake
// reasons sharing one string value, undetected. This test fails on any
// RunReasonXxx/
// legacyRunReasonXxx raw-literal declaration found outside
// internal/office/shared; every consumer must alias shared's constant
// instead (see shared/runreasons.go).
func TestWakeReasonRegistry_DeclaresEveryReasonOnlyInSharedPackage(t *testing.T) {
	literals, walkErr := walkRunReasonLiterals(t)
	if walkErr != nil {
		t.Fatalf("walk internal/office: %v", walkErr)
	}

	var outside []string
	for _, lit := range literals {
		if isUnderSharedPackage(lit.file) {
			continue
		}
		outside = append(outside, fmt.Sprintf("%s = %q (declared in %s)", lit.name, lit.value, lit.file))
	}
	if len(outside) > 0 {
		t.Errorf("wake-reason constant(s) declared as a raw literal outside internal/office/shared:\n%s\n"+
			"AC-OFFICE-BACKPRESSURE-001.8 requires internal/office/shared to be the single "+
			"declaration site. Declare the canonical constant in shared/runreasons.go and "+
			"alias it here instead (e.g. RunReasonTaskAssigned = shared.RunReasonTaskAssigned).",
			strings.Join(outside, "\n"))
	}
}

// isUnderSharedPackage reports whether an absolute file path is inside
// internal/office/shared (any file in that package, not just
// wakereasons.go/runreasons.go — the canonical declaration site can live
// in either).
func isUnderSharedPackage(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/internal/office/shared/")
}

// walkRunReasonLiterals parses every non-test .go file under
// internal/office and returns every top-level RunReasonXxx/
// legacyRunReasonXxx const declaration whose value is a raw string
// literal (not an alias of another identifier), with its declaring file.
func walkRunReasonLiterals(t *testing.T) ([]runReasonLiteral, error) {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve internal/office root: %v", err)
	}

	var literals []runReasonLiteral
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
		for _, lit := range declaredRunReasonLiterals(file) {
			literals = append(literals, runReasonLiteral{name: lit.name, value: lit.value, file: path})
		}
		return nil
	})
	return literals, walkErr
}

// namedLiteral pairs a const identifier with its raw string literal value.
type namedLiteral struct {
	name  string
	value string
}

// declaredRunReasonLiterals returns every top-level const in file whose
// identifier starts with "RunReason" or "legacyRunReason" and whose value
// is itself a raw string literal (not an alias of another identifier).
func declaredRunReasonLiterals(file *ast.File) []namedLiteral {
	var reasons []namedLiteral
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
				reasons = append(reasons, namedLiteral{name: name.Name, value: value})
			}
		}
	}
	return reasons
}

func isRunReasonIdent(name string) bool {
	return strings.HasPrefix(name, "RunReason") || strings.HasPrefix(name, "legacyRunReason")
}
