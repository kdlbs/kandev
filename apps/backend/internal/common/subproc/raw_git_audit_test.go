package subproc

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestProductionGitCommandsUseTheAdmissionSeam is a repository guard for the
// global Git cap. Production code may construct Git commands only in the
// subproc seam; every other package must invoke a classified helper instead.
// This catches raw command construction, executable lookup, and direct exec
// additions that a search for the legacy subproc.Git accessor would miss.
func TestProductionGitCommandsUseTheAdmissionSeam(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	backendRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../../.."))
	var violations []string
	err := filepath.WalkDir(backendRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(backendRoot, path)
		if err != nil {
			return err
		}
		// The common/subproc package is the sole Git execution seam. It may
		// contain the platform-specific command/exec primitives, while every
		// production caller outside it must use a classified runner.
		if !strings.HasPrefix(filepath.ToSlash(rel), "internal/common/subproc/") {
			violations = append(violations, scanGoFile(t, path)...)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk production Go files: %v", err)
	}
	sort.Strings(violations)
	if len(violations) != 0 {
		t.Fatalf("raw Git execution bypasses the admission seam:\n%s", strings.Join(violations, "\n"))
	}
}

func scanGoFile(t *testing.T, path string) []string {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var violations []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		execPkg, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if (execPkg.Name == "unix" || execPkg.Name == "syscall") && selector.Sel.Name == "Exec" {
			position := fileSet.Position(call.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d: direct exec", filepath.ToSlash(path), position.Line))
			return true
		}
		if execPkg.Name != "exec" {
			return true
		}
		if selector.Sel.Name == "LookPath" {
			if len(call.Args) == 0 || !isGitString(call.Args[0]) {
				return true
			}
			position := fileSet.Position(call.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d: Git lookup", filepath.ToSlash(path), position.Line))
			return true
		}
		if selector.Sel.Name != "Command" && selector.Sel.Name != "CommandContext" {
			return true
		}
		// The command argument is the first argument for Command and the
		// second for CommandContext. Inspect both positions explicitly.
		commandIndex := 0
		if selector.Sel.Name == "CommandContext" {
			commandIndex = 1
		}
		if len(call.Args) <= commandIndex {
			return true
		}
		if !isGitString(call.Args[commandIndex]) {
			return true
		}
		position := fileSet.Position(call.Pos())
		violations = append(violations, fmt.Sprintf("%s:%d", filepath.ToSlash(path), position.Line))
		return true
	})
	return violations
}

func isGitString(expr ast.Expr) bool {
	// NOTE: this guard intentionally recognizes direct string literals only;
	// keep production Git construction on the shared subproc seam rather than
	// relying on constant aliases that this lightweight AST walk cannot resolve.
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return false
	}
	value, err := strconv.Unquote(literal.Value)
	return err == nil && value == "git"
}
