package testutil

import (
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
