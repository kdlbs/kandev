package testutil

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func parseSnippet(t *testing.T, src string) (*token.FileSet, *ast.File) {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "snippet.go", src, 0)
	if err != nil {
		t.Fatalf("parse snippet: %v", err)
	}
	return fileSet, file
}

func TestUncoveredEnvReadsCoveredLiteral(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

import "os"

func read() string { return os.Getenv("FOO") }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, []string{"FOO"}, nil)
	if len(messages) != 0 {
		t.Fatalf("covered literal reported as uncovered: %v", messages)
	}
}

func TestUncoveredEnvReadsCoveredConstant(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

import "os"

const fooEnv = "FOO"

func read() string { return os.Getenv(fooEnv) }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, []string{"FOO"}, nil)
	if len(messages) != 0 {
		t.Fatalf("covered constant reported as uncovered: %v", messages)
	}
}

func TestUncoveredEnvReadsUncoveredName(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

import "os"

func read() string { return os.Getenv("BAR") }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, nil, nil)
	if len(messages) != 1 {
		t.Fatalf("expected exactly one uncovered-name message, got %v", messages)
	}
	if !strings.Contains(messages[0], "BAR") {
		t.Fatalf("message %q does not name the uncovered variable", messages[0])
	}
}

// TestUncoveredEnvReadsUncoveredLookupEnv pins the os.LookupEnv half of the
// classifier. Neither guarded package reads the environment that way today, so
// dropping LookupEnv from isEnvRead would leave every other test in this file
// and both live package guards green while silently narrowing the scan.
func TestUncoveredEnvReadsUncoveredLookupEnv(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

import "os"

func read() (string, bool) { return os.LookupEnv("BAR") }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, nil, nil)
	if len(messages) != 1 {
		t.Fatalf("expected exactly one uncovered-name message, got %v", messages)
	}
	if !strings.Contains(messages[0], "BAR") {
		t.Fatalf("message %q does not name the uncovered variable", messages[0])
	}
}

func TestUncoveredEnvReadsUnresolvableIdentifier(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

import "os"

func read(name string) string { return os.Getenv(name) }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, []string{"FOO"}, nil)
	if len(messages) != 1 {
		t.Fatalf("expected exactly one unresolvable-identifier message, got %v", messages)
	}
	if !strings.Contains(messages[0], "not a string literal") {
		t.Fatalf("message %q does not explain the resolution failure", messages[0])
	}
}

// TestUncoveredEnvReadsIgnoresExtraReaderWhenNotDeclared pins the blind spot
// that makes the extraReaders parameter necessary: a package reading the
// environment through its own accessor is invisible to a Getenv-only scan.
func TestUncoveredEnvReadsIgnoresExtraReaderWhenNotDeclared(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

type op struct{}

func (o *op) environmentValue(key string) string { return "" }

func read(o *op) string { return o.environmentValue("BAR") }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, nil, nil)
	if len(messages) != 0 {
		t.Fatalf("undeclared accessor should not be scanned, got %v", messages)
	}
}

func TestUncoveredEnvReadsUncoveredExtraReaderMethod(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

type op struct{}

func (o *op) environmentValue(key string) string { return "" }

func read(o *op) string { return o.environmentValue("BAR") }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, nil, nil, "environmentValue")
	if len(messages) != 1 {
		t.Fatalf("expected exactly one uncovered-name message, got %v", messages)
	}
	if !strings.Contains(messages[0], "BAR") {
		t.Fatalf("message %q does not name the uncovered variable", messages[0])
	}
}

func TestUncoveredEnvReadsCoveredExtraReaderConstant(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

type op struct{}

const fooEnv = "FOO"

func (o *op) environmentValue(key string) string { return "" }

func read(o *op) string { return o.environmentValue(fooEnv) }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, []string{"FOO"}, nil, "environmentValue")
	if len(messages) != 0 {
		t.Fatalf("covered accessor read reported as uncovered: %v", messages)
	}
}

// TestUncoveredEnvReadsExtraReaderAsPlainFunction covers the package-level
// spelling of an extra reader, which matches on the name alone.
func TestUncoveredEnvReadsExtraReaderAsPlainFunction(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

func environmentValue(key string) string { return "" }

func read() string { return environmentValue("BAR") }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, nil, nil, "environmentValue")
	if len(messages) != 1 {
		t.Fatalf("expected exactly one uncovered-name message, got %v", messages)
	}
	if !strings.Contains(messages[0], "BAR") {
		t.Fatalf("message %q does not name the uncovered variable", messages[0])
	}
}

// TestUncoveredEnvReadsExtraReaderWithFallbackArgument pins the arity
// tolerance of the extra-reader match. The scan used to require exactly one
// argument, so giving an accessor a second parameter removed every one of its
// call sites from the scan at once, silently and with no test failing. Proven
// against the real package: with a variadic parameter added to
// (*GitOperator).environmentValue and an uncovered name read through it, the
// process guard reported ok while the exactly-one-argument rule stood, and
// failed naming git_pr_providers.go only once it was relaxed. A covered name
// cannot show this, since it reports ok whether it is scanned or skipped.
func TestUncoveredEnvReadsExtraReaderWithFallbackArgument(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

type op struct{}

func (o *op) environmentValue(key, fallback string) string { return fallback }

func read(o *op) string { return o.environmentValue("BAR", "") }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, nil, nil, "environmentValue")
	if len(messages) != 1 {
		t.Fatalf("expected exactly one uncovered-name message, got %v", messages)
	}
	if !strings.Contains(messages[0], "BAR") {
		t.Fatalf("message %q does not name the uncovered variable", messages[0])
	}
}

// TestUncoveredEnvReadsIgnoresNonOsSelector pins the package qualifier: a
// Getenv that belongs to some other package is not an environment read, and
// reporting it would be a false alarm the author cannot silence.
func TestUncoveredEnvReadsIgnoresNonOsSelector(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

import "example.com/cfg"

func read() string { return cfg.Getenv("BAR") }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, nil, nil)
	if len(messages) != 0 {
		t.Fatalf("non-os Getenv should not be scanned, got %v", messages)
	}
}

// TestUncoveredEnvReadsVarHeldNameIsUnresolvable pins the deliberate choice to
// resolve only constants. A name held in a var could change at runtime, so the
// guard refuses to classify it and says so instead of guessing.
func TestUncoveredEnvReadsVarHeldNameIsUnresolvable(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

import "os"

var fooEnv = "FOO"

func read() string { return os.Getenv(fooEnv) }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, []string{"FOO"}, nil)
	if len(messages) != 1 {
		t.Fatalf("expected exactly one unresolvable-identifier message, got %v", messages)
	}
	if !strings.Contains(messages[0], "not a string literal") {
		t.Fatalf("message %q does not explain the resolution failure", messages[0])
	}
}

// TestUncoveredEnvReadsStaleExtraReaderIsReported pins the rename protection.
// Extra readers are matched by name, so renaming the accessor leaves the string
// here compiling and matching nothing, and the guard goes back to watching only
// os.Getenv. Proven against the real package: renaming
// (*GitOperator).environmentValue to envValue package-wide and adding an
// uncovered read through it left the process guard reporting ok, which is
// exactly the blind spot extraReaders was added to close.
func TestUncoveredEnvReadsStaleExtraReaderIsReported(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

type op struct{}

// Renamed away from environmentValue; the guard's string was not updated.
func (o *op) envValue(key string) string { return "" }

func read(o *op) string { return o.envValue("FOO") }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, []string{"FOO"}, nil, "environmentValue")
	if len(messages) != 1 {
		t.Fatalf("expected exactly one stale-extra-reader message, got %v", messages)
	}
	if !strings.Contains(messages[0], "environmentValue") {
		t.Fatalf("message %q does not name the stale reader", messages[0])
	}
	if !strings.Contains(messages[0], "matches no call") {
		t.Fatalf("message %q does not explain that the reader matched nothing", messages[0])
	}
}

// TestUncoveredEnvReadsLiveExtraReaderIsNotReportedStale is the other half of
// the pair: a reader that is genuinely called must not be reported, or the
// staleness check would fire on every correct call site.
func TestUncoveredEnvReadsLiveExtraReaderIsNotReportedStale(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

type op struct{}

func (o *op) environmentValue(key string) string { return "" }

func read(o *op) string { return o.environmentValue("FOO") }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, []string{"FOO"}, nil, "environmentValue")
	if len(messages) != 0 {
		t.Fatalf("live extra reader reported: %v", messages)
	}
}

// parseSnippets parses several files into one file set, for the cases that only
// arise across files of a package rather than inside one.
func parseSnippets(t *testing.T, srcs ...string) (*token.FileSet, []*ast.File) {
	t.Helper()
	fileSet := token.NewFileSet()
	files := make([]*ast.File, 0, len(srcs))
	for i, src := range srcs {
		file, err := parser.ParseFile(fileSet, fmt.Sprintf("snippet%d.go", i), src, 0)
		if err != nil {
			t.Fatalf("parse snippet %d: %v", i, err)
		}
		files = append(files, file)
	}
	return fileSet, files
}

// TestUncoveredEnvReadsConflictingConstantIsUnresolvable pins the build-tag
// blind spot. Every file is parsed regardless of its tags, so a platform pair
// may legally declare one constant name twice with different values. Flattening
// those into a single map meant the later file's value won and the earlier
// file's read was classified against the wrong variable — silently, since a
// read of an unscrubbed name resolved to an exempt one reports ok. Proven
// against the real package: a //go:build !windows file reading
// os.Getenv(qaShellEnv) where qaShellEnv is "KANDEV_QA_UNIXONLY", paired with a
// //go:build windows file declaring the same name as "SHELL", left the process
// guard reporting ok even though the uncovered read is the one that compiles on
// Linux. The same probe now fails.
func TestUncoveredEnvReadsConflictingConstantIsUnresolvable(t *testing.T) {
	fileSet, files := parseSnippets(t, `//go:build !windows

package example

import "os"

const shellEnv = "UNCOVERED_ON_UNIX"

func read() string { return os.Getenv(shellEnv) }
`, `//go:build windows

package example

const shellEnv = "SHELL"
`)

	messages := uncoveredEnvReads(fileSet, files, nil, []string{"SHELL"})
	if len(messages) != 1 {
		t.Fatalf("expected exactly one conflicting-constant message, got %v", messages)
	}
	if !strings.Contains(messages[0], "shellEnv") {
		t.Fatalf("message %q does not name the conflicting constant", messages[0])
	}
	if !strings.Contains(messages[0], "different values in more than one file") {
		t.Fatalf("message %q does not explain the conflict", messages[0])
	}
}

// TestUncoveredEnvReadsRepeatedConstantWithSameValueStillResolves is the other
// half: a platform pair that declares one name twice with the SAME value is not
// ambiguous, and rejecting it would make the conflict check fire on a correct
// package.
func TestUncoveredEnvReadsRepeatedConstantWithSameValueStillResolves(t *testing.T) {
	fileSet, files := parseSnippets(t, `//go:build !windows

package example

import "os"

const shellEnv = "SHELL"

func read() string { return os.Getenv(shellEnv) }
`, `//go:build windows

package example

const shellEnv = "SHELL"
`)

	messages := uncoveredEnvReads(fileSet, files, nil, []string{"SHELL"})
	if len(messages) != 0 {
		t.Fatalf("unambiguous repeated constant reported: %v", messages)
	}
}

// TestUncoveredEnvReadsConstantResolvesAcrossFiles guards the capability the
// conflict check must not break: constants are package-scoped, so a read in one
// file resolves against a constant declared in another.
func TestUncoveredEnvReadsConstantResolvesAcrossFiles(t *testing.T) {
	fileSet, files := parseSnippets(t, `package example

import "os"

func read() string { return os.Getenv(fooEnv) }
`, `package example

const fooEnv = "FOO"
`)

	messages := uncoveredEnvReads(fileSet, files, []string{"FOO"}, nil)
	if len(messages) != 0 {
		t.Fatalf("cross-file constant reported as uncovered: %v", messages)
	}
}

// TestUncoveredEnvReadsUnreadableConstantDoesNotBorrowSiblingValue pins the
// other half of the build-tag collision, which the conflicting-literal check
// alone does not catch. A platform pair may declare one constant name twice with
// only ONE side written as a string literal — the other referring to a second
// constant, concatenating, or repeating the previous expression list. Two
// differing literals conflict, but a literal paired with an unreadable
// declaration did not, so the literal file's value was lent to the read in the
// file this scanner cannot read, exactly as silently. Proven against the real
// package: a //go:build !windows file declaring qaEnv = qaOtherEnv (where
// qaOtherEnv is the uncovered "KANDEV_QA_UNCOVERED") and reading os.Getenv(qaEnv),
// paired with a //go:build windows file declaring qaEnv = "SHELL", left the
// process guard reporting ok even though the uncovered read is the one that
// compiles on Linux. The same probe now fails.
func TestUncoveredEnvReadsUnreadableConstantDoesNotBorrowSiblingValue(t *testing.T) {
	fileSet, files := parseSnippets(t, `//go:build !windows

package example

import "os"

const otherEnv = "UNCOVERED_ON_UNIX"

const shellEnv = otherEnv

func read() string { return os.Getenv(shellEnv) }
`, `//go:build windows

package example

const shellEnv = "SHELL"
`)

	messages := uncoveredEnvReads(fileSet, files, nil, []string{"SHELL"})
	if len(messages) != 1 {
		t.Fatalf("expected exactly one unresolvable-identifier message, got %v", messages)
	}
	// The generic message, not the conflicting-literals one: nothing here was
	// declared with two different values, so pointing at that cause would send
	// the author looking for a second literal that does not exist.
	if !strings.Contains(messages[0], "not a string literal") {
		t.Fatalf("message %q does not explain the resolution failure", messages[0])
	}
}

// TestUncoveredEnvReadsImplicitRepetitionDoesNotBorrowSiblingValue covers the
// valueless spelling of the same hazard: in a const block, a name with no
// expression repeats the previous one, which this scanner does not evaluate.
func TestUncoveredEnvReadsImplicitRepetitionDoesNotBorrowSiblingValue(t *testing.T) {
	fileSet, files := parseSnippets(t, `//go:build !windows

package example

import "os"

const (
	firstEnv = "UNCOVERED_ON_UNIX"
	shellEnv
)

func read() string { return os.Getenv(shellEnv) }
`, `//go:build windows

package example

const shellEnv = "SHELL"
`)

	messages := uncoveredEnvReads(fileSet, files, nil, []string{"SHELL"})
	if len(messages) != 1 {
		t.Fatalf("expected exactly one unresolvable-identifier message, got %v", messages)
	}
	if !strings.Contains(messages[0], "not a string literal") {
		t.Fatalf("message %q does not explain the resolution failure", messages[0])
	}
}

// TestUncoveredEnvReadsUnreadableSpecDoesNotPoisonItsBlock bounds the rule
// above: a name is marked unresolvable individually, not by association with a
// const block or a multi-name spec that happens to contain one. Both guarded
// packages declare their env names beside only other string literals —
// git_pr_providers.go's block is literals throughout — so nothing live would
// notice if a refactor hoisted the marking to spec or block level, and every
// constant in a mixed block would start reporting unresolvable at once.
//
// Both hoists are covered, and each needs its own shape. fooEnv is a
// single-name spec beside an unreadable sibling spec, which only a block-level
// hoist poisons. bazEnv shares one spec with an unreadable name, which a
// spec-level hoist poisons too — the level a first mutation attempt reached for
// and found inert, because every other declaration here is one name per spec.
func TestUncoveredEnvReadsUnreadableSpecDoesNotPoisonItsBlock(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

import "os"

const timeout = 30

const (
	fooEnv       = "FOO"
	scale        = timeout
	bazEnv, unit = "BAZ", timeout
)

func read() string { return os.Getenv(fooEnv) + os.Getenv(bazEnv) }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, []string{"FOO", "BAZ"}, nil)
	if len(messages) != 0 {
		t.Fatalf("literal siblings of an unreadable spec reported as unresolvable: %v", messages)
	}
}

func TestUncoveredEnvReadsExemptName(t *testing.T) {
	fileSet, file := parseSnippet(t, `package example

import "os"

func shell() string { return os.Getenv("SHELL") }
`)

	messages := uncoveredEnvReads(fileSet, []*ast.File{file}, nil, []string{"SHELL"})
	if len(messages) != 0 {
		t.Fatalf("exempt name reported as uncovered: %v", messages)
	}
}
