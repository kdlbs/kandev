package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// @covers AC-WORKSPACES-BRANCH-INVENTORY-001.1, AC-WORKSPACES-BRANCH-INVENTORY-001.2
// @covers AC-WORKSPACES-BRANCH-INVENTORY-001.3, AC-WORKSPACES-BRANCH-INVENTORY-001.4
func TestListBranchesWithCurrent_DistinctBranchIdentities(t *testing.T) {
	isolateGitEnvForTest(t)
	cases := []struct {
		name   string
		local  string
		locals []Branch
	}{
		{
			name: "collision", local: "remotes/origin/topic",
			locals: []Branch{{Name: "main", Type: "local"}, {Name: "remotes/origin/topic", Type: "local"}},
		},
		{
			name: "control", local: "feature/topic",
			locals: []Branch{{Name: "feature/topic", Type: "local"}, {Name: "main", Type: "local"}},
		},
	}
	for _, storage := range []string{"loose", "packed"} {
		for _, tc := range cases {
			t.Run(storage+"/"+tc.name, func(t *testing.T) {
				fixture := newBranchInventoryFixture(t, tc.local)
				if storage == "packed" {
					inventoryGit(t, fixture.path, "pack-refs", "--all", "--prune")
				}
				fixture.assertTips(t)
				assertServiceBranchInventory(t, fixture.path, inventoryWithRemotes(tc.locals), tc.local)
			})
		}
	}
}

// @covers AC-WORKSPACES-BRANCH-INVENTORY-001.1, AC-WORKSPACES-BRANCH-INVENTORY-001.2
// @covers AC-WORKSPACES-BRANCH-INVENTORY-001.3
func TestListBranchesWithCurrent_MixedBranchStorage(t *testing.T) {
	isolateGitEnvForTest(t)
	fixture := newBranchInventoryFixture(t, "remotes/origin/topic")
	inventoryGit(t, fixture.path, "pack-refs", "--all", "--prune")
	packed, err := os.ReadFile(filepath.Join(fixture.path, ".git", "packed-refs"))
	if err != nil {
		t.Fatal(err)
	}
	for ref, tip := range map[string]string{
		"refs/heads/remotes/origin/topic": fixture.localTip,
		"refs/remotes/origin/topic":       fixture.remoteTip,
	} {
		if !strings.Contains(string(packed), tip+" "+ref+"\n") {
			t.Fatalf("packed fixture lacks %s at %s", ref, tip)
		}
	}
	inventoryGit(t, fixture.path, "symbolic-ref", "HEAD", "refs/heads/main")
	inventoryGit(t, fixture.path, "commit", "--allow-empty", "-m", "remote loose tip")
	fixture.remoteTip = inventoryGit(t, fixture.path, "rev-parse", "--verify", "HEAD")
	inventoryGit(t, fixture.path, "update-ref", "refs/remotes/origin/topic", fixture.remoteTip)
	inventoryGit(t, fixture.path, "symbolic-ref", "HEAD", "refs/heads/remotes/origin/topic")
	inventoryGit(t, fixture.path, "commit", "--allow-empty", "-m", "local loose tip")
	fixture.localTip = inventoryGit(t, fixture.path, "rev-parse", "--verify", "HEAD")
	fixture.assertTips(t)
	for ref, tip := range map[string]string{
		"refs/heads/remotes/origin/topic": fixture.localTip,
		"refs/remotes/origin/topic":       fixture.remoteTip,
	} {
		loose, readErr := os.ReadFile(filepath.Join(fixture.path, ".git", filepath.FromSlash(ref)))
		if readErr != nil || strings.TrimSpace(string(loose)) != tip {
			t.Fatalf("loose override %s = %q, err %v, want %s", ref, loose, readErr, tip)
		}
	}
	assertServiceBranchInventory(t, fixture.path, inventoryWithRemotes([]Branch{
		{Name: "main", Type: "local"},
		{Name: "remotes/origin/topic", Type: "local"},
	}), "remotes/origin/topic")
}

// @covers AC-WORKSPACES-BRANCH-INVENTORY-001.3, AC-WORKSPACES-BRANCH-INVENTORY-001.4
func TestListBranchesWithCurrent_LinkedInventory(t *testing.T) {
	isolateGitEnvForTest(t)
	fixture := newBranchInventoryFixture(t, "remotes/origin/topic")
	linkedPath := filepath.Join(t.TempDir(), "linked")
	inventoryGit(t, fixture.path, "worktree", "add", "-b", "linked/topic", linkedPath, "refs/heads/main")
	inventoryGit(t, fixture.path, "pack-refs", "--all", "--prune")
	fixture.assertTips(t)
	want := inventoryWithRemotes([]Branch{
		{Name: "linked/topic", Type: "local"},
		{Name: "main", Type: "local"},
		{Name: "remotes/origin/topic", Type: "local"},
	})
	t.Run("primary", func(t *testing.T) {
		assertServiceBranchInventory(t, fixture.path, want, "remotes/origin/topic")
	})
	t.Run("linked", func(t *testing.T) {
		assertServiceBranchInventory(t, linkedPath, want, "linked/topic")
	})
}

type branchInventoryFixture struct {
	path      string
	localName string
	localTip  string
	remoteTip string
}

func newBranchInventoryFixture(t *testing.T, localName string) branchInventoryFixture {
	t.Helper()
	repoPath := filepath.Join(t.TempDir(), "repository")
	initRealGitRepo(t, repoPath)
	localTip := inventoryGit(t, repoPath, "rev-parse", "--verify", "refs/heads/main")
	inventoryGit(t, repoPath, "commit", "--allow-empty", "-m", "remote tip")
	remoteTip := inventoryGit(t, repoPath, "rev-parse", "--verify", "refs/heads/main")
	for ref, tip := range map[string]string{
		"refs/heads/" + localName:     localTip,
		"refs/remotes/origin/topic":   remoteTip,
		"refs/remotes/origin/zeta":    localTip,
		"refs/remotes/upstream/alpha": remoteTip,
		"refs/remotes/upstream/HEAD":  localTip,
	} {
		inventoryGit(t, repoPath, "update-ref", ref, tip)
	}
	inventoryGit(t, repoPath, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/topic")
	// Every fixture commit has the same empty tree, so moving HEAD selects the
	// local branch without changing index or worktree contents.
	inventoryGit(t, repoPath, "symbolic-ref", "HEAD", "refs/heads/"+localName)
	return branchInventoryFixture{path: repoPath, localName: localName, localTip: localTip, remoteTip: remoteTip}
}

func (f branchInventoryFixture) assertTips(t *testing.T) {
	t.Helper()
	if f.localTip == f.remoteTip {
		t.Fatal("fixture local and remote tips must differ")
	}
	for ref, want := range map[string]string{
		"refs/heads/" + f.localName: f.localTip,
		"refs/remotes/origin/topic": f.remoteTip,
	} {
		if got := inventoryGit(t, f.path, "rev-parse", "--verify", ref); got != want {
			t.Fatalf("fixture %s = %s, want %s", ref, got, want)
		}
	}
}

func inventoryWithRemotes(locals []Branch) []Branch {
	return append(locals,
		Branch{Name: "topic", Type: "remote", Remote: "origin"},
		Branch{Name: "zeta", Type: "remote", Remote: "origin"},
		Branch{Name: "alpha", Type: "remote", Remote: "upstream"},
	)
}

func assertServiceBranchInventory(t *testing.T, repoPath string, want []Branch, current string) {
	t.Helper()
	svc := newDiscoveryService(t, t.TempDir())
	repoID := persistFreshBranchRepository(t, svc, repoPath)
	for _, source := range []string{"path", "repository"} {
		t.Run(source, func(t *testing.T) {
			before := inventoryGitState(t, repoPath)
			id, path := "", repoPath
			if source == "repository" {
				id, path = repoID, ""
			}
			got, err := svc.ListBranchesWithCurrent(context.Background(), id, path)
			if err != nil {
				t.Fatalf("ListBranchesWithCurrent: %v", err)
			}
			if !reflect.DeepEqual(got.Branches, want) {
				t.Errorf("branch inventory = %+v, want %+v", got.Branches, want)
			}
			if got.CurrentBranch != current {
				t.Errorf("CurrentBranch = %q, want %q", got.CurrentBranch, current)
			}
			if after := inventoryGitState(t, repoPath); after != before {
				t.Errorf("listing changed repository state: before %q, after %q", before, after)
			}
		})
	}
}

func inventoryGitState(t *testing.T, repoPath string) string {
	t.Helper()
	return inventoryGit(t, repoPath, "show-ref") + "\n" +
		inventoryGit(t, repoPath, "symbolic-ref", "HEAD") + "\n" +
		inventoryGit(t, repoPath, "rev-parse", "--verify", "HEAD") + "\n" +
		inventoryGit(t, repoPath, "rev-list", "--all")
}

func inventoryGit(t *testing.T, repoPath string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = repoPath
	cmd.Env = isolatedGitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
