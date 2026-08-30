package github

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ambientGitHubEnvVars lists every environment variable this package reads in
// non-test code. TestMain clears all of them before any test runs.
//
// Developer and CI shells commonly export GH_TOKEN/GITHUB_TOKEN so the gh CLI
// and other tooling work. Left in place they silently change auth-method
// resolution under test: legacyEnvironmentToken picks up the ambient token
// and newLegacyCredential returns a PAT client instead of the "no auth
// configured" NoopClient, which fails TestNewClient_NoAuth_ReturnsNoop and
// TestClearToken. KANDEV_MOCK_GITHUB is included for the same reason gitlab's
// mock switch is: an ambient "true" would silently swap in the mock client.
//
// Clearing once here rather than per test also sidesteps t.Setenv, which
// cannot be called from a test that uses t.Parallel().
var ambientGitHubEnvVars = []string{
	"GH_TOKEN",
	"GITHUB_TOKEN",
	"KANDEV_MOCK_GITHUB",
}

// clearAmbientGitHubEnv removes the inherited values so tests observe an
// unconfigured environment. Individual tests that need one of these variables
// still set it explicitly with t.Setenv.
func clearAmbientGitHubEnv() {
	for _, name := range ambientGitHubEnvVars {
		if err := os.Unsetenv(name); err != nil {
			panic("github tests: unset " + name + ": " + err.Error())
		}
	}
}

func TestAmbientGitHubEnvIsClearedForTests(t *testing.T) {
	// Report only the name: one of these variables holds a real token on a
	// developer machine and test output ends up in CI logs.
	for _, name := range ambientGitHubEnvVars {
		if _, ok := os.LookupEnv(name); ok {
			t.Errorf("%s is set during tests; TestMain must clear it", name)
		}
	}
}

// TestAmbientGitHubEnvCoversEveryPackageEnvRead fails when non-test code grows
// a new os.Getenv/os.LookupEnv call that ambientGitHubEnvVars does not cover,
// so the scrub cannot silently fall behind the code it protects.
func TestAmbientGitHubEnvCoversEveryPackageEnvRead(t *testing.T) {
	fileSet := token.NewFileSet()
	files := parseNonTestSources(t, fileSet)
	constants := stringConstants(files)
	covered := make(map[string]bool, len(ambientGitHubEnvVars))
	for _, name := range ambientGitHubEnvVars {
		covered[name] = true
	}
	for _, file := range files {
		for _, call := range envReadCalls(file) {
			name, resolved := resolveEnvName(call, constants)
			if !resolved {
				t.Errorf("%s: environment variable name is not a string literal or a constant of this "+
					"package; a constant imported from elsewhere needs resolveEnvName taught about it",
					fileSet.Position(call.Pos()))
				continue
			}
			if !covered[name] {
				t.Errorf("%s: %s is read in non-test code but missing from ambientGitHubEnvVars",
					fileSet.Position(call.Pos()), name)
			}
		}
	}
}

func TestEnvReadCallsResolvesOSImportAlias(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "aliased.go", `package github

import stdos "os"

func readToken() string {
	return stdos.Getenv("GH_TOKEN")
}
`, 0)
	if err != nil {
		t.Fatalf("parse aliased source: %v", err)
	}

	if got := len(envReadCalls(file)); got != 1 {
		t.Fatalf("envReadCalls() found %d calls, want 1", got)
	}
}

// parseNonTestSources parses every production file of the package, which is
// the code whose environment reads the scrub has to keep up with.
func parseNonTestSources(t *testing.T, fileSet *token.FileSet) []*ast.File {
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
// env reads written as os.Getenv(envSomeConst) resolve to the variable name.
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

func osPackageName(file *ast.File) (string, bool) {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || path != "os" {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name, spec.Name.Name != "_" && spec.Name.Name != "."
		}
		return "os", true
	}
	return "", false
}

// envReadCalls collects every os.Getenv / os.LookupEnv call in a file.
func envReadCalls(file *ast.File) []*ast.CallExpr {
	osName, ok := osPackageName(file)
	if !ok {
		return nil
	}
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
		if !ok || pkgIdent.Name != osName {
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
