package testutil

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// AssertEnvReadsCovered parses every non-test *.go file in the caller's
// package directory (the current working directory when the test runs) and
// fails t for any os.Getenv/os.LookupEnv call whose argument name is not
// present in scrubbed or exempt, or cannot be resolved to a string literal or
// same-package constant. Callers use it to guard a hermetic-environment test
// scrub against silently falling behind new environment reads.
func AssertEnvReadsCovered(t testing.TB, scrubbed, exempt []string) {
	t.Helper()
	fileSet := token.NewFileSet()
	files := parseNonTestSources(t, fileSet)
	for _, message := range uncoveredEnvReads(fileSet, files, scrubbed, exempt) {
		t.Error(message)
	}
}

// uncoveredEnvReads returns one message per os.Getenv/os.LookupEnv call in
// files whose argument cannot be resolved to a string literal or
// same-package constant, or whose resolved name is in neither scrubbed nor
// exempt. Kept independent of testing.TB so it can be driven directly from
// inline source snippets in tests.
func uncoveredEnvReads(fileSet *token.FileSet, files []*ast.File, scrubbed, exempt []string) []string {
	constants := stringConstants(files)
	covered := make(map[string]bool, len(scrubbed)+len(exempt))
	for _, name := range scrubbed {
		covered[name] = true
	}
	for _, name := range exempt {
		covered[name] = true
	}
	var messages []string
	for _, file := range files {
		messages = append(messages, uncoveredEnvReadsInFile(fileSet, file, constants, covered)...)
	}
	return messages
}

func uncoveredEnvReadsInFile(fileSet *token.FileSet, file *ast.File, constants map[string]string, covered map[string]bool) []string {
	var messages []string
	for _, call := range envReadCalls(file) {
		name, resolved := resolveEnvName(call, constants)
		pos := fileSet.Position(call.Pos())
		if !resolved {
			messages = append(messages, fmt.Sprintf("%s: environment variable name is not a string literal or a "+
				"resolvable same-package constant; pass a literal or constant so AssertEnvReadsCovered can classify it", pos))
			continue
		}
		if !covered[name] {
			messages = append(messages, fmt.Sprintf("%s: %s is read in non-test code but missing from the "+
				"scrubbed/exempt lists passed to AssertEnvReadsCovered", pos, name))
		}
	}
	return messages
}

// parseNonTestSources parses every production file in the current directory,
// which is the code whose environment reads a scrub has to keep up with.
func parseNonTestSources(t testing.TB, fileSet *token.FileSet) []*ast.File {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	var files []*ast.File
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		t.Fatal("no non-test sources found in the package directory")
	}
	return files
}

// stringConstants maps package-level string constant names to their values so
// env reads written as os.Getenv(someConst) resolve to the variable name.
func stringConstants(files []*ast.File) map[string]string {
	constants := make(map[string]string)
	for _, file := range files {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				continue
			}
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range valueSpec.Names {
					if i >= len(valueSpec.Values) {
						continue
					}
					if value, ok := literalString(valueSpec.Values[i]); ok {
						constants[name.Name] = value
					}
				}
			}
		}
	}
	return constants
}

// envReadCalls collects every os.Getenv / os.LookupEnv call in a file.
func envReadCalls(file *ast.File) []*ast.CallExpr {
	var calls []*ast.CallExpr
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := selector.X.(*ast.Ident)
		if !ok || pkgIdent.Name != "os" {
			return true
		}
		if selector.Sel.Name == "Getenv" || selector.Sel.Name == "LookupEnv" {
			calls = append(calls, call)
		}
		return true
	})
	return calls
}

func resolveEnvName(call *ast.CallExpr, constants map[string]string) (string, bool) {
	if value, ok := literalString(call.Args[0]); ok {
		return value, true
	}
	ident, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return "", false
	}
	value, ok := constants[ident.Name]
	return value, ok
}

func literalString(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}
