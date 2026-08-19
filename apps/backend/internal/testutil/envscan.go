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
// fails t for any environment read whose argument name is not present in
// scrubbed or exempt, or cannot be resolved to a string literal or
// same-package constant. Callers use it to guard a hermetic-environment test
// scrub against silently falling behind new environment reads.
//
// os.Getenv and os.LookupEnv are always treated as environment reads. They are
// matched on the literal identifier os, so a file that imports os under an
// alias or through a dot import is not scanned; no file in this repository
// does either, and a review of one that did would be the place to catch it.
// Names are likewise resolved against package-level constants with no scope
// analysis, so a local variable shadowing one of those constants resolves to
// the constant's value rather than its own.
//
// Only reads that name a variable are classified. An argument-less bulk read
// such as os.Environ, or an accessor wrapping one (filterGitEnv(g.environmentValues())
// in the process package), hands out the whole ambient environment with no name
// to check, so this guard says nothing about it; a scrub list is not the tool
// for that channel. Reads through an API other than os.Getenv/os.LookupEnv,
// syscall.Getenv among them, are likewise not scanned.
//
// A package that funnels its reads through its own accessor (for example
// (*GitOperator).environmentValue, which falls back to os.Environ) must name
// that accessor in extraReaders, or the guard sees only the minority of its
// reads and the assurance is hollow. Names in extraReaders match on the
// selector alone, so any receiver counts, as does a plain package-level
// function of the same name. Only the first argument is read, so an accessor
// that later grows a second parameter stays covered rather than silently
// dropping every one of its call sites out of the scan. For the same reason an
// extraReader matching no call at all is an error rather than a no-op: the
// match is by name, so renaming the accessor would otherwise leave the guard
// compiling, passing, and watching nothing.
func AssertEnvReadsCovered(t testing.TB, scrubbed, exempt []string, extraReaders ...string) {
	t.Helper()
	fileSet := token.NewFileSet()
	files := parseNonTestSources(t, fileSet)
	for _, message := range uncoveredEnvReads(fileSet, files, scrubbed, exempt, extraReaders...) {
		t.Error(message)
	}
}

// envScan carries the classification inputs shared by every file in one pass.
type envScan struct {
	constants    packageConstants
	covered      map[string]bool
	extraReaders map[string]bool
}

// packageConstants holds every package-level string constant, plus the names no
// single value can be attributed to. Build-tagged platform variants (foo_unix.go
// and foo_windows.go) may legally declare one name twice, and every file is
// parsed regardless of its tags, so a plain name-to-value map would silently
// keep whichever file was parsed last and classify the other file's read against
// the wrong variable.
//
// Two spellings of that hazard are tracked separately because they need
// different advice. conflicts holds names declared with two different string
// literals. unresolved holds names declared somewhere this scanner cannot read a
// literal from at all — a reference to another constant, a concatenation, an
// untyped iota, or an implicit repetition of the previous expression list. The
// second kind matters for the same reason as the first: without it a literal
// declaration in one build-tagged file lends its value to a same-named
// declaration in another that this scanner cannot read, and the read resolves to
// the wrong platform's variable just as silently.
type packageConstants struct {
	values     map[string]string
	conflicts  map[string]bool
	unresolved map[string]bool
}

func newPackageConstants() packageConstants {
	return packageConstants{
		values:     make(map[string]string),
		conflicts:  make(map[string]bool),
		unresolved: make(map[string]bool),
	}
}

// record keeps the first value seen for name and marks the name conflicting if
// a later file declares it with a different one. Re-declaring the same value is
// not a conflict: both spellings resolve to the same variable.
func (c packageConstants) record(name, value string) {
	if previous, seen := c.values[name]; seen && previous != value {
		c.conflicts[name] = true
		return
	}
	c.values[name] = value
}

// recordUnresolvable marks a name this scanner cannot read a string literal
// from, so no other file's declaration of the same name can stand in for it.
func (c packageConstants) recordUnresolvable(name string) {
	c.unresolved[name] = true
}

// resolve refuses to guess for a name that is conflicting or unreadable, so the
// read is reported as unclassifiable rather than silently resolved to whichever
// declaration this scanner happened to understand.
func (c packageConstants) resolve(name string) (string, bool) {
	if c.conflicts[name] || c.unresolved[name] {
		return "", false
	}
	value, ok := c.values[name]
	return value, ok
}

// uncoveredEnvReads returns one message per environment read in files whose
// argument cannot be resolved to a string literal or same-package constant,
// or whose resolved name is in neither scrubbed nor exempt. Kept independent
// of testing.TB so it can be driven directly from inline source snippets in
// tests.
func uncoveredEnvReads(fileSet *token.FileSet, files []*ast.File, scrubbed, exempt []string, extraReaders ...string) []string {
	scan := envScan{
		constants:    stringConstants(files),
		covered:      make(map[string]bool, len(scrubbed)+len(exempt)),
		extraReaders: make(map[string]bool, len(extraReaders)),
	}
	for _, name := range scrubbed {
		scan.covered[name] = true
	}
	for _, name := range exempt {
		scan.covered[name] = true
	}
	for _, name := range extraReaders {
		scan.extraReaders[name] = true
	}
	messages := staleExtraReaders(files, extraReaders)
	for _, file := range files {
		messages = append(messages, uncoveredEnvReadsInFile(fileSet, file, scan)...)
	}
	return messages
}

// staleExtraReaders returns one message per caller-declared extra reader that
// no scanned call in the package targets. Extra readers are matched by name, so
// the compiler cannot follow a rename of the accessor: the string keeps
// compiling, nothing fails, and the guard silently stops watching every read
// that went through it — the blind spot extraReaders exists to close, reopened
// by an ordinary refactor. A reader that matches nothing is therefore an error.
func staleExtraReaders(files []*ast.File, extraReaders []string) []string {
	var messages []string
	for _, name := range extraReaders {
		if callsExtraReader(files, name) {
			continue
		}
		messages = append(messages, fmt.Sprintf("extra reader %q passed to AssertEnvReadsCovered matches no call in "+
			"this package; if the accessor was renamed, rename it here too, because the guard stops watching "+
			"the reads that went through it", name))
	}
	return messages
}

// callsExtraReader reports whether any call the scan would look at targets
// name, in either the receiver.method(...) or the bare function(...) spelling.
// Argument-less calls are ignored to match envReadCalls, so "used" means
// "actually scanned" rather than merely "mentioned".
func callsExtraReader(files []*ast.File, name string) bool {
	found := false
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			switch target := call.Fun.(type) {
			case *ast.SelectorExpr:
				found = found || target.Sel.Name == name
			case *ast.Ident:
				found = found || target.Name == name
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func uncoveredEnvReadsInFile(fileSet *token.FileSet, file *ast.File, scan envScan) []string {
	var messages []string
	for _, call := range envReadCalls(file, scan.extraReaders) {
		name, resolved := resolveEnvName(call, scan.constants)
		pos := fileSet.Position(call.Pos())
		if !resolved {
			messages = append(messages, unresolvedMessage(pos, call.Args[0], scan.constants))
			continue
		}
		if !scan.covered[name] {
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

// unresolvedMessage explains why an environment name could not be classified.
// A name declared with different values in more than one file gets its own
// message: the cause is build-tagged platform variants, all of which this guard
// parses, and the generic "pass a literal or constant" advice would not lead an
// author there.
func unresolvedMessage(pos token.Position, arg ast.Expr, constants packageConstants) string {
	if ident, ok := arg.(*ast.Ident); ok && constants.conflicts[ident.Name] {
		return fmt.Sprintf("%s: constant %s is declared with different values in more than one file of this "+
			"package, most likely build-tagged platform variants, and every file is scanned regardless of its "+
			"tags, so the guard cannot tell which value applies here; read the variable through a string literal "+
			"instead", pos, ident.Name)
	}
	return fmt.Sprintf("%s: environment variable name is not a string literal or a resolvable same-package "+
		"constant; pass a literal or constant so AssertEnvReadsCovered can classify it", pos)
}

// stringConstants maps package-level string constant names to their values so
// env reads written as os.Getenv(someConst) resolve to the variable name, and
// records the names whose value would otherwise depend on which file you
// believe: those declared twice with different literals, and those declared
// anywhere in a form this scanner cannot read a literal from.
func stringConstants(files []*ast.File) packageConstants {
	constants := newPackageConstants()
	for _, file := range files {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				continue
			}
			recordConstSpecs(constants, genDecl.Specs)
		}
	}
	return constants
}

// recordConstSpecs records every string-literal constant in one const block, and
// marks every other name it declares unresolvable. Skipping those instead would
// let a same-named literal in a build-tagged sibling file resolve this read.
func recordConstSpecs(constants packageConstants, specs []ast.Spec) {
	for _, spec := range specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range valueSpec.Names {
			if i < len(valueSpec.Values) {
				if value, ok := literalString(valueSpec.Values[i]); ok {
					constants.record(name.Name, value)
					continue
				}
			}
			constants.recordUnresolvable(name.Name)
		}
	}
}

// envReadCalls collects every os.Getenv / os.LookupEnv call in a file, plus
// every call to one of the caller-declared extraReaders. Any call carrying at
// least one argument qualifies, because resolveEnvName only ever reads the
// first: pinning this to exactly one argument meant that adding a fallback
// parameter to an accessor dropped all of its call sites from the scan without
// failing a single test.
func envReadCalls(file *ast.File, extraReaders map[string]bool) []*ast.CallExpr {
	var calls []*ast.CallExpr
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		if isEnvRead(call.Fun, extraReaders) {
			calls = append(calls, call)
		}
		return true
	})
	return calls
}

// isEnvRead reports whether fun names os.Getenv, os.LookupEnv, or one of the
// extraReaders. An extraReader matches on the trailing name alone, so both
// receiver.environmentValue(x) and a bare environmentValue(x) count.
func isEnvRead(fun ast.Expr, extraReaders map[string]bool) bool {
	switch target := fun.(type) {
	case *ast.SelectorExpr:
		if extraReaders[target.Sel.Name] {
			return true
		}
		pkgIdent, ok := target.X.(*ast.Ident)
		if !ok || pkgIdent.Name != "os" {
			return false
		}
		return target.Sel.Name == "Getenv" || target.Sel.Name == "LookupEnv"
	case *ast.Ident:
		return extraReaders[target.Name]
	default:
		return false
	}
}

func resolveEnvName(call *ast.CallExpr, constants packageConstants) (string, bool) {
	if value, ok := literalString(call.Args[0]); ok {
		return value, true
	}
	ident, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return "", false
	}
	return constants.resolve(ident.Name)
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
