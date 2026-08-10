package gitlab

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

// envMockGitLab mirrors the mock switch that factory.go and service_config.go
// read as a bare literal. TestAmbientGitLabEnvCoversEveryPackageEnvRead
// resolves literals as well as constants, so the two cannot drift apart
// unnoticed.
const envMockGitLab = "KANDEV_MOCK_GITLAB"

// ambientGitLabEnvVars lists every environment variable this package reads in
// non-test code. TestMain clears all of them before any test runs.
//
// Developer shells commonly set these for real: GITLAB_HOST is the standard
// variable for the glab CLI, and GITLAB_TOKEN is its companion PAT. Left in
// place they silently change host and credential resolution under test — a set
// GITLAB_HOST makes trustedEnvironmentTokenHost ignore the startup host, which
// fails TestEnvironmentTokenAllowsImmutableStartupHost with a trusted-origin
// rejection, and KANDEV_MOCK_GITLAB=true swaps the real client for the mock in
// a dozen controller tests.
//
// Clearing once here rather than per test also sidesteps t.Setenv, which
// cannot be called from a test that uses t.Parallel().
var ambientGitLabEnvVars = []string{
	envKandevGitLabHost,
	envGitLabHost,
	envMockGitLab,
	secretNameToken,
}

// clearAmbientGitLabEnv removes the inherited values so tests observe an
// unconfigured environment. Individual tests that need one of these variables
// still set it explicitly with t.Setenv.
func clearAmbientGitLabEnv() {
	for _, name := range ambientGitLabEnvVars {
		if err := os.Unsetenv(name); err != nil {
			panic("gitlab tests: unset " + name + ": " + err.Error())
		}
	}
}

func TestAmbientGitLabEnvIsClearedForTests(t *testing.T) {
	// Report only the name: one of these variables holds a real PAT on a
	// developer machine and test output ends up in CI logs.
	for _, name := range ambientGitLabEnvVars {
		if _, ok := os.LookupEnv(name); ok {
			t.Errorf("%s is set during tests; TestMain must clear it", name)
		}
	}
}

// TestTrustedEnvironmentTokenHostFallsBackToStartupHost pins the resolution
// TestEnvironmentTokenAllowsImmutableStartupHost depends on: with no host
// variable in the environment, the immutable startup host is the trusted
// origin. Production precedence is unchanged — an explicitly set variable
// still wins — so this fails only when the ambient environment leaks back in.
func TestTrustedEnvironmentTokenHostFallsBackToStartupHost(t *testing.T) {
	const startupHost = "https://gitlab.internal"
	if got := trustedEnvironmentTokenHost(startupHost); got != startupHost {
		t.Fatalf("trustedEnvironmentTokenHost(%q) = %q, want the startup host", startupHost, got)
	}
}

// TestClearAmbientGitLabEnvNeutralizesInheritedHosts reproduces the original
// failure in-process: a shell exporting GITLAB_HOST (or KANDEV_GITLAB_HOST)
// hijacks the trusted origin, and the scrub takes it back. Production
// precedence is deliberately left intact — an explicitly configured variable
// still outranks the startup host — so the guard asserts the scrub, not the
// precedence.
func TestClearAmbientGitLabEnvNeutralizesInheritedHosts(t *testing.T) {
	const startupHost = "https://gitlab.internal"
	t.Setenv(envGitLabHost, "https://attacker.invalid")
	t.Setenv(envKandevGitLabHost, "https://evil.invalid")
	if got := trustedEnvironmentTokenHost(startupHost); got == startupHost {
		t.Fatal("inherited host variables no longer affect the trusted origin; this guard is obsolete")
	}

	clearAmbientGitLabEnv()

	if got := trustedEnvironmentTokenHost(startupHost); got != startupHost {
		t.Fatalf("after scrub trustedEnvironmentTokenHost(%q) = %q, want the startup host", startupHost, got)
	}
}

// TestAmbientGitLabEnvCoversEveryPackageEnvRead fails when non-test code grows
// a new os.Getenv/os.LookupEnv call that ambientGitLabEnvVars does not cover,
// so the scrub cannot silently fall behind the code it protects.
func TestAmbientGitLabEnvCoversEveryPackageEnvRead(t *testing.T) {
	fileSet := token.NewFileSet()
	files := parseNonTestSources(t, fileSet)
	constants := stringConstants(files)
	covered := make(map[string]bool, len(ambientGitLabEnvVars))
	for _, name := range ambientGitLabEnvVars {
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
				t.Errorf("%s: %s is read in non-test code but missing from ambientGitLabEnvVars",
					fileSet.Position(call.Pos()), name)
			}
		}
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
// env reads written as os.Getenv(envGitLabHost) resolve to the variable name.
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
