package process

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// @covers AC-WORKSPACES-SYMLINK-001.1, AC-WORKSPACES-SYMLINK-001.3
func TestSymlinkStatusMetadata(t *testing.T) {
	for _, scenario := range []string{"untracked", "staged", "unstaged", "deleted", "staged-deleted", "staged-delete-recreate", "renamed", "mixed-type"} {
		t.Run(scenario, func(t *testing.T) {
			repo, cleanup := setupTestRepo(t)
			defer cleanup()
			name := "link file.txt"
			link := filepath.Join(repo, name)
			if err := os.Symlink("missing-target", link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			name = prepareSymlinkScenario(t, repo, name, scenario)
			tracker := NewWorkspaceTracker(repo, newTestLogger(t))
			status, err := tracker.getGitStatus(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal(status.Files[name])
			if err != nil {
				t.Fatal(err)
			}
			var file struct {
				IsSymlink *bool `json:"is_symlink"`
				Staged    *struct {
					IsSymlink *bool `json:"is_symlink"`
				} `json:"staged_change"`
				Unstaged *struct {
					IsSymlink *bool `json:"is_symlink"`
				} `json:"unstaged_change"`
			}
			if err := json.Unmarshal(b, &file); err != nil {
				t.Fatal(err)
			}
			want := scenario != "mixed-type" && scenario != "staged-delete-recreate"
			if file.IsSymlink == nil || *file.IsSymlink != want {
				t.Fatalf("symlink metadata = %s, want %v", b, want)
			}
			if (scenario == "mixed-type" || scenario == "staged-delete-recreate") && (file.Staged == nil || file.Unstaged == nil || file.Staged.IsSymlink == nil || !*file.Staged.IsSymlink || file.Unstaged.IsSymlink == nil || *file.Unstaged.IsSymlink) {
				t.Fatalf("mixed layer metadata = %s", b)
			}
		})
	}
}

func prepareSymlinkScenario(t *testing.T, repo, name, scenario string) string {
	t.Helper()
	link := filepath.Join(repo, name)
	if scenario != "untracked" {
		runGit(t, repo, "add", "--", name)
	}
	if scenario == "untracked" || scenario == "staged" {
		return name
	}
	runGit(t, repo, "commit", "-m", "add symlink")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	switch scenario {
	case "unstaged":
		if err := os.Symlink("another-missing-target", link); err != nil {
			t.Fatal(err)
		}
	case "staged-deleted":
		runGit(t, repo, "add", "--", name)
	case "staged-delete-recreate":
		runGit(t, repo, "add", "--", name)
		writeFile(t, repo, name, "regular now\n")
	case "renamed":
		name = "renamed link.txt"
		if err := os.Symlink("missing-target", filepath.Join(repo, name)); err != nil {
			t.Fatal(err)
		}
		runGit(t, repo, "add", "-A")
	case "mixed-type":
		if err := os.Symlink("new-target", link); err != nil {
			t.Fatal(err)
		}
		runGit(t, repo, "add", "--", name)
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		writeFile(t, repo, name, "regular now\n")
	}
	return name
}

func TestSymlinkRawModes(t *testing.T) {
	raw := ":100644 120000 abc def T\x00tab\tand\nnewline\x00:120000 000000 abc def D\x00deleted\x00:120000 100644 abc def T\x00regular\x00:100644 000000 abc def U\x00conflict\x00"
	modes := parseChangedSymlinkModes(raw)
	if len(modes) != 3 || !modes["tab\tand\nnewline"] || !modes["deleted"] || modes["regular"] {
		t.Fatalf("modes = %#v", modes)
	}
	if symlinkModeValue(modes, "unknown") != nil {
		t.Fatal("missing metadata must remain unknown")
	}
}

func TestSymlinkMetadataWithoutPatch(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()
	external := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(external, []byte("private target"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(repo, "external")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	runGit(t, repo, "add", "external")
	tracker := NewWorkspaceTracker(repo, newTestLogger(t))
	update := types.GitStatusUpdate{Files: map[string]types.FileInfo{"external": {Path: "external", Status: "added", Staged: true, DiffSkipReason: "budget_exceeded"}}}
	tracker.enrichSymlinkMetadata(context.Background(), &update)
	file := update.Files["external"]
	if file.IsSymlink == nil || !*file.IsSymlink || file.Diff != "" || file.DiffSkipReason != "budget_exceeded" {
		t.Fatalf("file = %+v", file)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if modes := tracker.readChangedSymlinkModes(ctx, true); modes != nil {
		t.Fatalf("cancelled metadata read = %#v", modes)
	}
}
