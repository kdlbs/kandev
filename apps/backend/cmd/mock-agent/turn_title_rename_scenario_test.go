package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type titleRenameFakeAgent struct {
	result   string
	err      error
	texts    []string
	onCall   func()
	callArgs map[string]any
}

func (f *titleRenameFakeAgent) text(message string) {
	f.texts = append(f.texts, message)
}

func (f *titleRenameFakeAgent) callMCPToolCtx(_ context.Context, server, tool string, args map[string]any) (string, error) {
	f.callArgs = args
	if server != "kandev" || tool != "set_task_title_kandev" {
		return "", errors.New("unexpected MCP tool")
	}
	if f.onCall != nil {
		f.onCall()
	}
	return f.result, f.err
}

func (f *titleRenameFakeAgent) textContains(fragment string) bool {
	for _, message := range f.texts {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func setupTitleRenameScenarioRepo(t *testing.T) string {
	t.Helper()
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
	for _, step := range [][]string{{"add", "README.md"}, {"commit", "-q", "-m", "base"}, {"checkout", "-q", "-b", "feature/provisional"}} {
		if err := git(step...); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(repo)
	return repo
}

func titleRenameResultJSON(t *testing.T, accepted bool, status, branch string) string {
	t.Helper()
	payload := map[string]any{
		"accepted": accepted,
		"branch_rename": map[string]any{
			"status":  status,
			"renamed": []map[string]any{{"repository_id": "repo-1", "from": "feature/provisional", "to": branch}},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestTurnTitleRenameScenarioRenameOnly(t *testing.T) {
	setupTitleRenameScenarioRepo(t)
	agent := &titleRenameFakeAgent{result: titleRenameResultJSON(t, true, "renamed", "feature/provisional")}
	runTurnTitleRenameScenario(agent, turnTitleRenameOnly)

	if agent.callArgs["title"] != turnTitleRenameTitle {
		t.Fatalf("title args = %#v, want %q", agent.callArgs, turnTitleRenameTitle)
	}
	if !agent.textContains(turnTitleRenameOnlyMarker) {
		t.Fatalf("scenario texts = %v, want completion marker", agent.texts)
	}
	if _, err := os.Stat(turnTitleRenameFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rename-only scenario created %s", turnTitleRenameFile)
	}
}

func TestTurnTitleRenameScenarioEditWritesAfterVerifiedRename(t *testing.T) {
	setupTitleRenameScenarioRepo(t)
	agent := &titleRenameFakeAgent{result: titleRenameResultJSON(t, true, "renamed", "feature/provisional")}
	agent.onCall = func() {
		if _, err := os.Stat(turnTitleRenameFile); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("scenario wrote the turn file before the title tool resolved")
		}
	}
	runTurnTitleRenameScenario(agent, turnTitleRenameWithEdit)

	content, err := os.ReadFile(turnTitleRenameFile)
	if err != nil || string(content) != turnTitleRenameContent {
		t.Fatalf("turn file = %q, %v; want exact edit content", content, err)
	}
	if !agent.textContains(turnTitleRenameEditMarker) {
		t.Fatalf("scenario texts = %v, want completion marker", agent.texts)
	}
}

func TestTurnTitleRenameScenarioSwitchKeepsHead(t *testing.T) {
	repo := setupTitleRenameScenarioRepo(t)
	headBefore, err := runGitOutput(repo, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	agent := &titleRenameFakeAgent{result: titleRenameResultJSON(t, true, "renamed", "feature/provisional")}
	runTurnTitleRenameScenario(agent, turnTitleRenameThenSwitch)

	branch, err := runGitOutput(repo, "branch", "--show-current")
	if err != nil || strings.TrimSpace(string(branch)) != turnTitleRenameSwitchName {
		t.Fatalf("branch = %q, %v; want %q", branch, err, turnTitleRenameSwitchName)
	}
	headAfter, err := runGitOutput(repo, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(string(headAfter)) != strings.TrimSpace(string(headBefore)) {
		t.Fatalf("HEAD = %q, %v; want unchanged %q", headAfter, err, headBefore)
	}
	if !agent.textContains(turnTitleRenameSwitchMarker) {
		t.Fatalf("scenario texts = %v, want completion marker", agent.texts)
	}
}

func TestTurnTitleRenameScenarioRequiresAcceptedVerifiedRename(t *testing.T) {
	tests := []struct {
		name   string
		result string
		err    error
	}{
		{name: "mcp failure", err: errors.New("mcp unavailable")},
		{name: "title rejected", result: titleRenameResultJSON(t, false, "renamed", "feature/provisional")},
		{name: "rename failed", result: titleRenameResultJSON(t, true, "failed", "feature/provisional")},
		{name: "rename preserved", result: titleRenameResultJSON(t, true, "preserved", "feature/provisional")},
		{name: "wrong branch", result: titleRenameResultJSON(t, true, "renamed", "feature/other")},
		{name: "malformed result", result: "not-json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupTitleRenameScenarioRepo(t)
			agent := &titleRenameFakeAgent{result: tc.result, err: tc.err}
			runTurnTitleRenameScenario(agent, turnTitleRenameWithEdit)
			if _, err := os.Stat(turnTitleRenameFile); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("scenario wrote %s without a verified rename", turnTitleRenameFile)
			}
			if agent.textContains(turnTitleRenameEditMarker) {
				t.Fatalf("scenario reported completion without a verified rename")
			}
			if !agent.textContains("turn-title-rename:") {
				t.Fatalf("scenario texts = %v, want explicit failure", agent.texts)
			}
		})
	}
}
