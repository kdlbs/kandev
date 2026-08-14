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
