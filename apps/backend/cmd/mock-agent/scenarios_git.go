package main

import (
	"fmt"
	"os"
	"strings"
)

const diffUpdateTestFile = "diff_update_test.txt"

// scenarioMultiTurn: minimal response for multi-turn test.
func scenarioMultiTurn(e *emitter) {
	fixedDelay(50)
	e.text("Multi-turn response ready. Send another message to continue.")
}

// scenarioDiffExpansionSetup: creates a committed file and then modifies it,
// leaving an uncommitted diff that the UI can display with expansion enabled.
func scenarioDiffExpansionSetup(e *emitter) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		e.text("diff-expansion-setup: getwd failed: " + err.Error())
		return
	}

	const totalLines = 200
	originalLines := make([]string, totalLines)
	for i := 0; i < totalLines; i++ {
		originalLines[i] = fmt.Sprintf("func original_%03d() { /* line %d */ }", i+1, i+1)
	}
	original := strings.Join(originalLines, "\n") + "\n"

	filePath := "expansion_test.go"
	if err := os.WriteFile(filePath, []byte(original), 0o644); err != nil {
		e.text("diff-expansion-setup: write failed: " + err.Error())
		return
	}

	runGitCmd := makeGitRunner(wd)
	// Add the canonical content directly. `--allow-empty` makes retries
	// idempotent when a reused worktree already has the same file at HEAD;
	// overwriting before `git add` also repairs a worktree left with the prior
	// scenario's modified content. The old rm/cleanup commit sequence could
	// fail under concurrent git setup and left the real fixture commit absent.
	if err := runGitCmd("add", filePath); err != nil {
		e.text("diff-expansion-setup: git add failed")
		return
	}
	if err := runGitCmd("commit", "--allow-empty", "-m", "add expansion_test.go for e2e diff expansion test"); err != nil {
		e.text("diff-expansion-setup: git commit failed")
		return
	}

	modifiedLines := make([]string, totalLines)
	copy(modifiedLines, originalLines)
	modifiedLines[49] = "func modified_mid_top() { /* HUNK_TOP - modified line 50 */ }"
	modifiedLines[149] = "func modified_mid_bottom() { /* HUNK_BOTTOM - modified line 150 */ }"
	modified := strings.Join(modifiedLines, "\n") + "\n"

	if err := os.WriteFile(filePath, []byte(modified), 0o644); err != nil {
		e.text("diff-expansion-setup: write modified failed: " + err.Error())
		return
	}

	fixedDelay(100)
	e.text("diff-expansion-setup complete: expansion_test.go has two-hunk uncommitted diff (lines 50 and 150 of 200).")
}

// scenarioDiffUpdateSetup creates a simple file, commits it, then modifies it.
func scenarioDiffUpdateSetup(e *emitter) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		e.text("diff-update-setup: getwd failed: " + err.Error())
		return
	}

	filePath := diffUpdateTestFile
	originalContent := "line 1: original\nline 2: unchanged\nline 3: original\n"

	if err := os.WriteFile(filePath, []byte(originalContent), 0o644); err != nil {
		e.text("diff-update-setup: write failed: " + err.Error())
		return
	}

	runGitCmd := makeGitRunner(wd)
	_ = runGitCmd("rm", "--force", filePath)
	_ = runGitCmd("commit", "-m", "cleanup diff_update_test.txt")

	if err := os.WriteFile(filePath, []byte(originalContent), 0o644); err != nil {
		e.text("diff-update-setup: re-write failed: " + err.Error())
		return
	}

	if err := runGitCmd("add", filePath); err != nil {
		e.text("diff-update-setup: git add failed")
		return
	}
	if err := runGitCmd("commit", "-m", "add diff_update_test.txt"); err != nil {
		e.text("diff-update-setup: git commit failed")
		return
	}

	modifiedContent := "line 1: FIRST_MODIFICATION\nline 2: unchanged\nline 3: original\n"
	if err := os.WriteFile(filePath, []byte(modifiedContent), 0o644); err != nil {
		e.text("diff-update-setup: write modified failed: " + err.Error())
		return
	}

	fixedDelay(100)
	e.text("diff-update-setup complete: diff_update_test.txt has FIRST_MODIFICATION")
}

// scenarioDiffUpdateModify modifies the diff_update_test.txt file again.
func scenarioDiffUpdateModify(e *emitter) {
	fixedDelay(50)

	filePath := diffUpdateTestFile
	modifiedContent := "line 1: SECOND_MODIFICATION\nline 2: unchanged\nline 3: ALSO_CHANGED\n"
	if err := os.WriteFile(filePath, []byte(modifiedContent), 0o644); err != nil {
		e.text("diff-update-modify: write failed: " + err.Error())
		return
	}

	fixedDelay(100)
	e.text("diff-update-modify complete: diff_update_test.txt now has SECOND_MODIFICATION")
}

// scenarioPushCurrentBranch pushes the worktree's current branch to origin
// directly, with no Kandev "Create MR"/"Create PR" action involved. Used by
// push-detection auto-link e2e tests to reproduce an agent running a raw
// `git push` on its own — the same shape as a real user/agent pushing and
// then opening the MR/PR through `glab mr create`, `gh pr create`, or the
// code host's web UI, rather than through Kandev's own runtime action.
// Expects a prior turn (e.g. diff-update-setup) to have already committed
// something to push.
//
// Explicitly writes the remote-tracking ref and upstream config after
// pushing rather than relying on `git push -u` alone: the e2e fixture's mock
// git remote only implements the receive-pack (push) side of the smart-HTTP
// protocol, not upload-pack/info-refs (fetch/clone) — confirmed by
// worktree-manager's own "git fetch failed before worktree creation;
// continuing with fallback ref" warning during task setup, and reproducible
// directly (`git fetch` against it fails with "not valid: is this a git
// repository?"). A real code host's push response updates
// refs/remotes/origin/<branch> as a side effect of the exchange; here that
// has to be reproduced by hand with `update-ref`, using the local HEAD this
// process just successfully pushed as the known-good value — no fetch
// required, since the pushed content is exactly what's already local.
func scenarioPushCurrentBranch(e *emitter) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		e.text("push-current-branch: getwd failed: " + err.Error())
		return
	}
	branchNameOut, err := runGitOutput(wd, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		e.text("push-current-branch: resolve branch name failed: " + err.Error())
		return
	}
	branchName := strings.TrimSpace(string(branchNameOut))
	headSHAOut, err := runGitOutput(wd, "rev-parse", "HEAD")
	if err != nil {
		e.text("push-current-branch: resolve HEAD sha failed: " + err.Error())
		return
	}
	headSHA := strings.TrimSpace(string(headSHAOut))

	runGitCmd := makeGitRunner(wd)
	if err := runGitCmd("push", "origin", branchName); err != nil {
		e.text("push-current-branch: git push failed")
		return
	}
	if err := runGitCmd("update-ref", "refs/remotes/origin/"+branchName, headSHA); err != nil {
		e.text("push-current-branch: git update-ref failed")
		return
	}
	if err := runGitCmd("branch", "--set-upstream-to=origin/"+branchName, branchName); err != nil {
		e.text("push-current-branch: git branch --set-upstream-to failed")
		return
	}

	fixedDelay(100)
	e.text("push-current-branch complete: pushed directly, no Create MR/PR action used")
}

// scenarioDiffUpdateStreaming modifies the file mid-turn, emitting text both
// before and after the write so the agent's turn stays active for several
// seconds. Used to assert that open file editor / diff panels auto-update
// while the agent is still streaming.
func scenarioDiffUpdateStreaming(e *emitter) {
	fixedDelay(50)
	e.text("diff-update-streaming: starting work")

	fixedDelay(1000)

	filePath := diffUpdateTestFile
	modifiedContent := "line 1: SECOND_MODIFICATION\nline 2: unchanged\nline 3: ALSO_CHANGED\n"
	if err := os.WriteFile(filePath, []byte(modifiedContent), 0o644); err != nil {
		e.text("diff-update-streaming: write failed: " + err.Error())
		return
	}

	fixedDelay(500)
	e.text("diff-update-streaming: file written, continuing")

	// Long tail keeps the turn active long enough for polling+sync to fire.
	fixedDelay(6000)
	e.text("diff-update-streaming complete")
}

// scenarioMultiFileSetup commits three tracked files, then modifies all three
// so each shows up in git status with FIRST_MODIFICATION. Used to test the
// case where the user has multiple file editors / diff panels open at once.
func scenarioMultiFileSetup(e *emitter) {
	fixedDelay(50)

	wd, err := os.Getwd()
	if err != nil {
		e.text("multi-file-setup: getwd failed: " + err.Error())
		return
	}

	runGitCmd := makeGitRunner(wd)
	files := []string{"multi_a.txt", "multi_b.txt", "multi_c.txt"}

	// Cleanup any leftovers from a prior run.
	for _, f := range files {
		_ = runGitCmd("rm", "--force", f)
	}
	_ = runGitCmd("commit", "-m", "cleanup multi-file fixtures")

	// Commit pristine versions.
	for _, f := range files {
		original := fmt.Sprintf("%s line 1: original\n%s line 2: unchanged\n%s line 3: original\n", f, f, f)
		if err := os.WriteFile(f, []byte(original), 0o644); err != nil {
			e.text("multi-file-setup: write failed: " + err.Error())
			return
		}
		if err := runGitCmd("add", f); err != nil {
			e.text("multi-file-setup: git add failed for " + f)
			return
		}
	}
	if err := runGitCmd("commit", "-m", "add multi-file fixtures"); err != nil {
		e.text("multi-file-setup: git commit failed")
		return
	}

	// Modify all three so each appears in git status with FIRST_MODIFICATION.
	for _, f := range files {
		modified := fmt.Sprintf("%s line 1: FIRST_MODIFICATION\n%s line 2: unchanged\n%s line 3: original\n", f, f, f)
		if err := os.WriteFile(f, []byte(modified), 0o644); err != nil {
			e.text("multi-file-setup: write modified failed: " + err.Error())
			return
		}
	}

	fixedDelay(100)
	e.text("multi-file-setup complete: 3 files have FIRST_MODIFICATION")
}

// scenarioMultiFileModify modifies all three multi-file fixtures within a
// single turn, with a long trailing delay so the panels must auto-update
// while the agent is still streaming. Reproduces the user's reported case
// where multiple open editor / diff panels go stale on a real edit.
func scenarioMultiFileModify(e *emitter) {
	fixedDelay(50)
	e.text("multi-file-modify: starting work")

	fixedDelay(800)

	files := []string{"multi_a.txt", "multi_b.txt", "multi_c.txt"}
	for i, f := range files {
		modified := fmt.Sprintf(
			"%s line 1: SECOND_MODIFICATION\n%s line 2: unchanged\n%s line 3: ALSO_CHANGED_%d\n",
			f, f, f, i,
		)
		if err := os.WriteFile(f, []byte(modified), 0o644); err != nil {
			e.text("multi-file-modify: write failed: " + err.Error())
			return
		}
		// Stagger the writes slightly to mimic a real agent doing one tool
		// call per file rather than all writes in a single instant.
		fixedDelay(250)
	}

	e.text("multi-file-modify: writes done, continuing")

	// Long tail keeps the turn active so we can assert mid-turn updates.
	fixedDelay(5000)
	e.text("multi-file-modify complete")
}

// scenarioUntrackedFileSetup creates a new untracked file.
func scenarioUntrackedFileSetup(e *emitter) {
	fixedDelay(50)

	filePath := "untracked_test.txt"
	content := "line 1: INITIAL_CONTENT\nline 2: some text\n"

	_ = os.Remove(filePath)

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		e.text("untracked-file-setup: write failed: " + err.Error())
		return
	}

	fixedDelay(100)
	e.text("untracked-file-setup complete: untracked_test.txt has INITIAL_CONTENT")
}

// scenarioUntrackedFileModify modifies the untracked file.
func scenarioUntrackedFileModify(e *emitter) {
	fixedDelay(50)

	filePath := "untracked_test.txt"
	content := "line 1: MODIFIED_CONTENT\nline 2: some text\nline 3: NEW_LINE\n"

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		e.text("untracked-file-modify: write failed: " + err.Error())
		return
	}

	fixedDelay(100)
	e.text("untracked-file-modify complete: untracked_test.txt now has MODIFIED_CONTENT")
}
