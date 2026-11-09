package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTurnHistoryIntegrityScenario(t *testing.T) {
	repo := t.TempDir()
	git := makeGitRunner(repo)
	if err := git("init", "--initial-branch=main"); err != nil {
		t.Fatal(err)
	}
	if err := git("config", "user.email", "mock@test.local"); err != nil {
		t.Fatal(err)
	}
	if err := git("config", "user.name", "Mock Agent"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := git("add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if err := git("commit", "-q", "-m", "base"); err != nil {
		t.Fatal(err)
	}
	if err := git("checkout", "-q", "-b", "feature/turn"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "untracked-before.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)

	emitter, updater := newTestEmitter()
	scenarioTurnHistoryIntegrity(emitter)

	updates := updater.getUpdates()
	found := false
	for _, update := range updates {
		if strings.Contains(getTextContent(update), "turn-history-integrity complete") {
			found = true
		}
	}
	if !found {
		t.Fatalf("scenario did not complete: %+v", updates)
	}
	if _, err := os.Stat(filepath.Join(repo, "turn-history.txt")); err != nil {
		t.Fatalf("turn file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "upstream-history.txt")); err != nil {
		t.Fatalf("upstream file missing: %v", err)
	}
	mergeBase, err := runGitOutput(repo, "merge-base", "HEAD", "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	head, err := runGitOutput(repo, "rev-parse", "refs/heads/main")
	if err != nil || strings.TrimSpace(string(mergeBase)) != strings.TrimSpace(string(head)) {
		t.Fatalf("rebase did not land on the updated base: %q vs %q, %v", mergeBase, head, err)
	}
}

func TestTurnHistoryConflictScenario(t *testing.T) {
	repo := t.TempDir()
	git := makeGitRunner(repo)
	for _, step := range [][]string{
		{"init", "--initial-branch=main"}, {"config", "user.email", "mock@test.local"},
		{"config", "user.name", "Mock Agent"},
	} {
		if err := git(step...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := git("add", "README.md"); err != nil {
		t.Fatal(err)
	}
	if err := git("commit", "-q", "-m", "base"); err != nil {
		t.Fatal(err)
	}
	if err := git("checkout", "-q", "-b", "feature/conflict"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)
	emitter, updater := newTestEmitter()
	scenarioTurnHistoryConflictSetup(emitter)
	scenarioTurnHistoryConflict(emitter)
	content, err := os.ReadFile(filepath.Join(repo, "README.md"))
	if err != nil || string(content) != "upstream conflict\n" {
		t.Fatalf("conflict scenario end content = %q, %v", content, err)
	}
	found := false
	for _, update := range updater.getUpdates() {
		if strings.Contains(getTextContent(update), "turn-history-conflict complete") {
			found = true
		}
	}
	if !found {
		t.Fatalf("conflict scenario did not complete: %+v", updater.getUpdates())
	}
}
