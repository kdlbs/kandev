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
