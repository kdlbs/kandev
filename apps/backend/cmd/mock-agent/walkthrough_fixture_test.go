package main

import "testing"

func TestWriteWalkthroughFileReusesCleanFixture(t *testing.T) {
	repo := t.TempDir()
	runGit := makeGitRunner(repo)
	t.Chdir(repo)
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "mock@test.local"},
		{"config", "user.name", "Mock Agent"},
		{"config", "commit.gpgsign", "false"},
	} {
		if err := runGit(args...); err != nil {
			t.Fatal(err)
		}
	}

	emitter, _ := newTestEmitter()
	const path = "walkthrough_range.txt"
	for run := range 2 {
		if !writeWalkthroughFile(emitter, runGit, "walkthrough fixture", path, "Anchor\n", "") {
			t.Fatalf("fixture setup failed on run %d", run+1)
		}
	}
}
