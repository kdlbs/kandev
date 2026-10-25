package subproc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// contentWritingSubcommands are the literal subcommands production callers use
// that the policy treats as content-writing (the default class).
var contentWritingSubcommands = setOf(
	"add", "commit", "checkout", "switch", "restore", "merge", "pull", "revert", "reset", "rebase",
	"cherry-pick", "worktree", "apply", "stash", "submodule", "am", "mv", "clone", "init", "branch",
	"tag", "rm", "check-ignore", "reflog", "clean", "credential", "sparse-checkout",
)

// spreadingCallFiles is the inventory of production files whose NewGitCommand
// calls build the argument list at run time. The run-time classification rule
// (an unlisted subcommand is content-writing) covers them; the inventory keeps
// a new such file from appearing unreviewed.
var spreadingCallFiles = setOf(
	"cmd/mock-agent/scenarios.go",
	"internal/agentctl/server/api/workspace_materialize.go",
	"internal/agentctl/server/process/git.go",
	"internal/agentctl/server/process/git_contribution_history.go",
	"internal/agentctl/server/process/manager_submodules.go",
	"internal/agentctl/server/process/turn_checkpoint_capture.go",
	"internal/agentctl/server/process/workspace_git_cmd.go",
	"internal/agentctl/server/process/workspace_git_diff.go",
	"internal/agent/runtime/lifecycle/env_preparer_local.go",
	"internal/backendapp/worktree.go",
	"internal/gitbootstrap/gitbootstrap.go",
	"internal/office/configloader/git.go",
	"internal/office/skills/materialization.go",
	"internal/office/testharness/routes.go",
	"internal/repoclone/clone.go",
	"internal/repoclone/remote_state.go",
	"internal/task/gitinit/command_path.go",
	"internal/task/handlers/executor_profile_handlers.go",
	"internal/task/inventoryrepair/git.go",
	"internal/task/service/fresh_branch.go",
	"internal/worktree/git_command.go",
)

func TestDirectGitCommandSubcommandsAreClassified(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("locate test package: %v", err)
	}
	backendRoot := filepath.Clean(filepath.Join(packageDir, "../../.."))
	var violations []string
	err = filepath.WalkDir(backendRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(backendRoot, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "internal/common/subproc/") {
			return nil
		}
		violations = append(violations, classificationViolations(t, path, rel)...)
		return nil
	})
	if err != nil {
		t.Fatalf("walk production Go files: %v", err)
	}
	sort.Strings(violations)
	if len(violations) != 0 {
		t.Fatalf("Git command subcommands are not classified:\n%s", strings.Join(violations, "\n"))
	}
}

func classificationViolations(t *testing.T, path, rel string) []string {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var violations []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "NewGitCommand" || len(call.Args) == 0 {
			return true
		}
		position := fileSet.Position(call.Pos())
		sub, literal := literalSubcommand(call.Args[1:], call.Ellipsis.IsValid())
		switch {
		case !literal && !spreadingCallFiles[rel]:
			violations = append(violations, position.String()+": run-time arguments in a file outside the spreading inventory")
		case literal && !knownSubcommand(sub):
			violations = append(violations, position.String()+": subcommand "+strconv.Quote(sub)+" is not classified")
		}
		return true
	})
	return violations
}

// literalSubcommand returns the first non-option argument when every argument
// up to and including it is a string literal.
func literalSubcommand(args []ast.Expr, spread bool) (string, bool) {
	if spread {
		return "", false
	}
	for index := 0; index < len(args); index++ {
		value, ok := stringLiteral(args[index])
		if !ok {
			return "", false
		}
		if value == "-C" || value == "-c" {
			index++
			continue
		}
		if strings.HasPrefix(value, "-") {
			continue
		}
		return value, true
	}
	return "", false
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

func knownSubcommand(sub string) bool {
	return sub == "cat-file" || filterFreeSubcommands[sub] || filterReadingSubcommands[sub] || contentWritingSubcommands[sub]
}
