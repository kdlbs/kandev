package main

import (
	"os"
	"path/filepath"
	"strings"
)

// scenarioTurnHistoryIntegrity performs a real upstream rebase during the
// admitted turn. The capture endpoints then contain imported history, while the
// accepted comparison should retain only the local turn file.
func scenarioTurnHistoryIntegrity(e *emitter) {
	wd, err := os.Getwd()
	if err != nil {
		e.text("turn-history-integrity: working directory unavailable")
		return
	}
	branch, targetRef, ok := turnHistoryBase(wd)
	if !ok {
		e.text("turn-history-integrity: stable comparison base unavailable")
		return
	}
	if err := rebaseTurnHistory(wd, branch, targetRef); err != nil {
		e.text("turn-history-integrity: " + err.Error())
		return
	}
	e.text("turn-history-integrity complete")
}

func turnHistoryBase(wd string) (string, string, bool) {
	branchOut, err := runGitOutput(wd, "branch", "--show-current")
	if err != nil {
		return "", "", false
	}
	branch := strings.TrimSpace(string(branchOut))
	for _, candidate := range []string{"refs/remotes/origin/main", "refs/heads/main"} {
		if _, err := runGitOutput(wd, "rev-parse", "--verify", candidate); err == nil {
			if candidate != "refs/heads/"+branch {
				return branch, candidate, true
			}
		}
	}
	return branch, "", false
}

func rebaseTurnHistory(wd, branch, targetRef string) error {
	baseOut, err := runGitOutput(wd, "rev-parse", targetRef)
	if err != nil {
		return err
	}
	base := strings.TrimSpace(string(baseOut))
	git := makeGitRunner(wd)
	if err := git("checkout", "-q", "-b", "kandev-history-upstream", base); err != nil {
		return err
	}
	if err := writeGitFile("upstream-history.txt", "integrated upstream change\n"); err != nil {
		return err
	}
	if err := git("add", "upstream-history.txt"); err != nil {
		return err
	}
	if err := git("commit", "-q", "-m", "upstream history advance"); err != nil {
		return err
	}
	upstreamOut, err := runGitOutput(wd, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if err := git("checkout", "-q", branch); err != nil {
		return err
	}
	if err := git("update-ref", targetRef, strings.TrimSpace(string(upstreamOut))); err != nil {
		return err
	}
	if err := writeGitFile("turn-history.txt", "local turn change\n"); err != nil {
		return err
	}
	if err := git("add", "turn-history.txt"); err != nil {
		return err
	}
	if err := git("commit", "-q", "-m", "local turn change"); err != nil {
		return err
	}
	if err := git("rebase", "kandev-history-upstream"); err != nil {
		return err
	}
	_ = git("branch", "-D", "kandev-history-upstream")
	return nil
}

func writeGitFile(path, content string) error {
	_ = os.Remove(path)
	return os.WriteFile(path, []byte(content), 0o644)
}

// scenarioTurnHistoryConflict leaves a starting local edit that conflicts with
// upstream history, so the accepted comparison must be explicitly unavailable.
func scenarioTurnHistoryConflict(e *emitter) {
	wd, err := os.Getwd()
	if err != nil {
		e.text("turn-history-conflict: working directory unavailable")
		return
	}
	branch, targetRef, ok := turnHistoryBase(wd)
	if !ok {
		e.text("turn-history-conflict: stable comparison base unavailable")
		return
	}
	if err := stageTurnHistoryConflict(wd, branch, targetRef); err != nil {
		e.text("turn-history-conflict: " + err.Error())
		return
	}
	e.text("turn-history-conflict complete")
}

func stageTurnHistoryConflict(wd, branch, targetRef string) error {
	baseOut, err := runGitOutput(wd, "rev-parse", targetRef)
	if err != nil {
		return err
	}
	base := strings.TrimSpace(string(baseOut))
	git := makeGitRunner(wd)
	if err := git("checkout", "-q", "-b", "kandev-history-conflict", base); err != nil {
		return err
	}
	if err := writeGitFile("README.md", "upstream conflict\n"); err != nil {
		return err
	}
	if err := git("add", "README.md"); err != nil {
		return err
	}
	if err := git("commit", "-q", "-m", "upstream conflict"); err != nil {
		return err
	}
	upstreamOut, err := runGitOutput(wd, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if err := git("checkout", "-q", branch); err != nil {
		return err
	}
	if err := git("update-ref", targetRef, strings.TrimSpace(string(upstreamOut))); err != nil {
		return err
	}
	if err := git("checkout", "-f", "-q", "-B", branch, strings.TrimSpace(string(upstreamOut))); err != nil {
		return err
	}
	_ = git("branch", "-D", "kandev-history-conflict")
	return nil
}

// scenarioTurnHistoryConflictSetup leaves a starting local edit for the next
// turn to conflict with upstream history.
func scenarioTurnHistoryConflictSetup(e *emitter) {
	wd, err := os.Getwd()
	if err != nil {
		e.text("turn-history-conflict-setup: working directory unavailable")
		return
	}
	if err := writeGitFile(filepath.Join(wd, "README.md"), "local conflict\n"); err != nil {
		e.text("turn-history-conflict-setup: write failed")
		return
	}
	e.text("turn-history-conflict-setup complete")
}
