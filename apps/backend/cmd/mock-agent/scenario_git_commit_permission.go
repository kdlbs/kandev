package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// permissionDeniedResult is the tool result recorded when a permission request
// is refused.
const permissionDeniedResult = "denied"

// gitCommitPermissionFile is the file the git-commit-permission scenario stages.
const gitCommitPermissionFile = "kandev-permission-probe.txt"

// scenarioGitCommitPermission asks for permission to run a state-changing Git
// command and, only when it is granted, actually creates the commit.
//
// It exists so the unattended-permission path can be accepted against a commit
// object that resolves, rather than against a displayed mode or the absence of
// an error message. Both directions matter: a profile that should run
// unattended must produce a commit with no human answering, and the default
// profile must leave the call pending until someone does.
func scenarioGitCommitPermission(e *emitter) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		e.text("git-commit-permission: getwd failed: " + err.Error())
		return
	}
	// Unique content per run: a repeated run over the same worktree would
	// otherwise stage nothing and `git commit` would fail with "nothing to
	// commit" rather than exercising the permission path.
	path := wd + string(os.PathSeparator) + gitCommitPermissionFile
	probe := fmt.Sprintf("kandev permission probe %d\n", time.Now().UnixNano())
	if err := os.WriteFile(path, []byte(probe), 0o644); err != nil {
		e.text("git-commit-permission: write probe file failed: " + err.Error())
		return
	}

	commitID := nextToolID()
	commitInput := map[string]any{
		rawInputCommandKey:     "git commit -m 'kandev permission probe'",
		subagentKeyDescription: "Commit the probe file",
	}
	e.startTool(commitID, "Run git commit", acp.ToolKindExecute, commitInput)

	allowed := e.requestPermission(commitID, "Run git commit", acp.ToolKindExecute, commitInput)
	fixedDelay(50)

	if !allowed {
		e.completeTool(commitID, map[string]any{toolKeyError: permissionDeniedResult})
		e.text("git-commit-permission: refused, no commit created.")
		return
	}

	if _, err := runGitOutput(wd, "add", gitCommitPermissionFile); err != nil {
		e.completeTool(commitID, map[string]any{toolKeyError: err.Error()})
		e.text("git-commit-permission: git add failed: " + err.Error())
		return
	}
	// The worktree carries no committer identity of its own, and a bare
	// `git commit` exits non-zero without one.
	if _, err := runGitOutput(wd,
		"-c", "user.name=Kandev Mock Agent",
		"-c", "user.email=mock-agent@kandev.invalid",
		"commit", "-m", "kandev permission probe",
	); err != nil {
		e.completeTool(commitID, map[string]any{toolKeyError: err.Error()})
		e.text("git-commit-permission: git commit failed: " + err.Error())
		return
	}
	shaOut, err := runGitOutput(wd, "rev-parse", "HEAD")
	if err != nil {
		e.completeTool(commitID, map[string]any{toolKeyError: err.Error()})
		e.text("git-commit-permission: resolve HEAD failed: " + err.Error())
		return
	}
	sha := strings.TrimSpace(string(shaOut))
	e.completeTool(commitID, map[string]any{rawOutputKey: sha})
	// The SHA is emitted so a test can resolve the commit object instead of
	// inferring success from the absence of an error.
	e.text("git-commit-permission: committed " + sha)
}
