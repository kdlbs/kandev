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
	return scanParsedGoFile(fileSet, path, file)
}

func scanGoSource(t *testing.T, filename, source string) []string {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, filename, source, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	return scanParsedGoFile(fileSet, filename, file)
}

func scanParsedGoFile(fileSet *token.FileSet, path string, file *ast.File) []string {
	var violations []string
	gitCommands := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		markGitCommandVariables(node, gitCommands)
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if command, ok := execPkgForGitCommand(selector, gitCommands); ok {
			position := fileSet.Position(call.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d: direct Git lifecycle %s", filepath.ToSlash(path), position.Line, command))
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

func markGitCommandVariables(node ast.Node, gitCommands map[string]bool) {
	switch statement := node.(type) {
	case *ast.FuncDecl:
		clear(gitCommands)
	case *ast.FuncLit:
		clear(gitCommands)
	case *ast.AssignStmt:
		for index, rhs := range statement.Rhs {
			if index >= len(statement.Lhs) {
				continue
			}
			identifier, ok := statement.Lhs[index].(*ast.Ident)
			if !ok {
				continue
			}
			gitCommands[identifier.Name] = isGitCommandConstructor(rhs)
		}
	case *ast.ValueSpec:
		for index, value := range statement.Values {
			if index >= len(statement.Names) {
				continue
			}
			gitCommands[statement.Names[index].Name] = isGitCommandConstructor(value)
		}
	}
}

func isGitCommandConstructor(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch function := call.Fun.(type) {
	case *ast.Ident:
		return function.Name == "newGitCommand" || function.Name == "newNonInteractiveGitCmd"
	case *ast.SelectorExpr:
		return function.Sel.Name == "NewGitCommand" || function.Sel.Name == "newNonInteractiveGitCmd"
	default:
		return false
	}
}

func execPkgForGitCommand(selector *ast.SelectorExpr, gitCommands map[string]bool) (string, bool) {
	if selector.Sel.Name != "Start" && selector.Sel.Name != "Wait" &&
		selector.Sel.Name != "Run" && selector.Sel.Name != "Output" &&
		selector.Sel.Name != "CombinedOutput" && selector.Sel.Name != "StdoutPipe" &&
		selector.Sel.Name != "StderrPipe" {
		return "", false
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok || !gitCommands[identifier.Name] {
		return "", false
	}
	return selector.Sel.Name, true
}

func TestScanGoSourceRejectsDirectGitLifecycle(t *testing.T) {
	violations := scanGoSource(t, "fixture.go", `package fixture

import (
	"context"
	"github.com/kandev/kandev/internal/common/subproc"
)

func run() {
	cmd := subproc.NewGitCommand(context.Background(), "status")
	_ = cmd.Start()
}
`)
	if len(violations) != 1 || !strings.Contains(violations[0], "direct Git lifecycle Start") {
		t.Fatalf("direct Git lifecycle violations = %v", violations)
	}
}

func TestScanGoSourceAllowsClassifiedGitLifecycle(t *testing.T) {
	violations := scanGoSource(t, "fixture.go", `package fixture

import (
	"context"
	"github.com/kandev/kandev/internal/common/subproc"
)

func run() error {
	cmd := subproc.NewGitCommand(context.Background(), "status")
	return subproc.RunGitClass(context.Background(), subproc.GitLifecycle, cmd)
}
`)
	if len(violations) != 0 {
		t.Fatalf("classified Git lifecycle violations = %v", violations)
	}
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
