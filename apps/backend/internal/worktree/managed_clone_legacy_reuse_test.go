package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type legacyReuseFixture struct {
	manager *Manager
	store   *managedCloneRelocationStore
	request RecoveryAdmissionRequest
	wt      *Worktree
	proof   *ManagedCloneRelocationProof
}

func newLegacyReuseFixture(t *testing.T, provider string, ownerLayout, mainCheckout bool) legacyReuseFixture {
	t.Helper()
	config := newTestConfig(t)
	root := filepath.Join(config.TasksBasePath, "repos")
	host := provider + ".com"
	source := filepath.Join(root, "_providers", provider, host, "acme", "widget")
	if ownerLayout {
		source = filepath.Join(root, "acme", "widget")
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
	seed := initGitRepoForWorktreeTest(t)
	runGit(t, seed, "clone", "--no-hardlinks", seed, source)
	configureManagedCloneRelocationGitIdentity(t, source)
	runGit(t, source, "remote", "set-url", "origin", "https://"+host+"/acme/widget.git")
	checkout := source
	branch := "main"
	if !mainCheckout {
		checkout = filepath.Join(config.TasksBasePath, "task-1", "widget")
		require.NoError(t, os.MkdirAll(filepath.Dir(checkout), 0o755))
		branch = "feature/legacy"
		runGit(t, source, "worktree", "add", "-b", branch, checkout)
	}
	wt := &Worktree{ID: "wt-legacy", TaskID: "task-1", TaskEnvironmentID: "env-1", RepositoryID: "repo-1", Path: checkout, RepositoryPath: source, Branch: branch, BranchSlug: "main", Status: StatusActive}
	proof := &ManagedCloneRelocationProof{ManagedRoot: root, ExpectedSourcePath: filepath.Join(root, "_providers", provider, host, "acme", "widget"), LegacyOwnerNameSourcePath: filepath.Join(root, "acme", "widget"), ExpectedDestinationPath: filepath.Join(root, "workspaces", "workspace-1", provider, "acme", "widget"), Identity: ManagedRepositoryIdentity{Provider: provider, Host: host, Owner: "acme", Name: "widget"}}
	store := &managedCloneRelocationStore{recoveryCASStore: &recoveryCASStore{mockStore: newMockStore()}}
	store.worktrees[wt.ID] = wt
	manager, err := NewManager(config, store, newTestLogger())
	require.NoError(t, err)
	request := RecoveryAdmissionRequest{TaskID: wt.TaskID, SessionID: "session-1", TaskEnvironmentID: wt.TaskEnvironmentID, OwnerTaskID: wt.TaskID, OwnershipGeneration: 1, ExecutorType: string(models.ExecutorTypeWorktree), Slots: []RecoverySlot{{WorktreeID: wt.ID, RepositoryID: wt.RepositoryID, BranchSlug: wt.BranchSlug, RepositoryPath: source, CloneRelocation: proof}}}
	return legacyReuseFixture{manager, store, request, wt, proof}
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.5
func TestManagerAdmitRecoveryReusesRegisteredLegacyClone(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		for _, ownerLayout := range []bool{false, true} {
			for _, mainCheckout := range []bool{false, true} {
				name := provider
				if ownerLayout {
					name += "/owner-layout"
				} else {
					name += "/provider-layout"
				}
				if mainCheckout {
					name += "/main"
				} else {
					name += "/linked"
				}
				t.Run(name, func(t *testing.T) {
					f := newLegacyReuseFixture(t, provider, ownerLayout, mainCheckout)
					before := runGit(t, f.wt.Path, "rev-parse", "HEAD")
					storedBefore := *f.wt
					admission, err := f.manager.AdmitRecovery(context.Background(), f.request)
					require.NoError(t, err)
					require.Nil(t, admission)
					require.Equal(t, before, runGit(t, f.wt.Path, "rev-parse", "HEAD"))
					require.Equal(t, storedBefore, *f.store.worktrees[storedBefore.ID])
					require.NoDirExists(t, f.proof.ExpectedDestinationPath)
					require.Nil(t, f.store.claim)
				})
			}
		}
	}
}

func addLegacyReuseContent(t *testing.T, f legacyReuseFixture) map[string][]byte {
	t.Helper()
	sub := initGitRepoForWorktreeTest(t)
	runGit(t, f.wt.Path, "-c", "protocol.file.allow=always", "submodule", "add", sub, "sub")
	runGit(t, f.wt.Path, "commit", "-m", "add submodule")
	require.NoError(t, os.WriteFile(filepath.Join(f.wt.Path, ".gitignore"), []byte("ignored.txt\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(f.wt.Path, "staged.txt"), []byte("staged\n"), 0o600))
	runGit(t, f.wt.Path, "add", "staged.txt")
	require.NoError(t, os.WriteFile(filepath.Join(f.wt.Path, "staged.txt"), []byte("unstaged\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(f.wt.Path, "ignored.txt"), []byte("build output\n"), 0o600))
	runGit(t, f.wt.Path, "config", "filter.test.smudge", "cat")
	runGit(t, f.wt.Path, "config", "filter.test.clean", "cat")
	require.NoError(t, os.WriteFile(filepath.Join(f.wt.Path, ".gitattributes"), []byte("filtered.txt filter=test\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(f.wt.Path, "filtered.txt"), []byte("filtered content\n"), 0o600))
	runGit(t, f.wt.Path, "add", ".gitattributes", "filtered.txt")
	require.Equal(t, "filtered.txt: filter: test", strings.TrimSpace(runGit(t, f.wt.Path, "check-attr", "filter", "--", "filtered.txt")))
	return captureLegacyReuseContent(t, f.wt.Path)
}

func captureLegacyReuseContent(t *testing.T, checkout string) map[string][]byte {
	t.Helper()
	snapshot := map[string][]byte{
		"filter":    []byte(runGit(t, checkout, "config", "--local", "--get-regexp", "^filter\\.test\\.")),
		"HEAD":      []byte(runGit(t, checkout, "rev-parse", "HEAD")),
		"branch":    []byte(runGit(t, checkout, "symbolic-ref", "HEAD")),
		"submodule": []byte(runGit(t, filepath.Join(checkout, "sub"), "rev-parse", "HEAD")),
	}
	for _, name := range []string{"staged.txt", "ignored.txt", ".gitignore", "sub/.git", ".gitattributes", "filtered.txt"} {
		data, err := os.ReadFile(filepath.Join(checkout, name))
		require.NoError(t, err)
		snapshot[name] = data
	}
	index := strings.TrimSpace(runGit(t, checkout, "rev-parse", "--git-path", "index"))
	if !filepath.IsAbs(index) {
		index = filepath.Join(checkout, index)
	}
	data, err := os.ReadFile(index)
	require.NoError(t, err)
	snapshot["index"] = data
	return snapshot
}

func TestManagerAdmitRecoveryPreservesLegacyCheckoutContent(t *testing.T) {
	for _, mainCheckout := range []bool{false, true} {
		for _, destinationExists := range []bool{false, true} {
			t.Run(fmt.Sprintf("main=%t/destination=%t", mainCheckout, destinationExists), func(t *testing.T) {
				f := newLegacyReuseFixture(t, "github", true, mainCheckout)
				before := addLegacyReuseContent(t, f)
				if destinationExists {
					require.NoError(t, os.MkdirAll(filepath.Dir(f.proof.ExpectedDestinationPath), 0o755))
					runGit(t, f.wt.RepositoryPath, "clone", "--no-hardlinks", f.wt.RepositoryPath, f.proof.ExpectedDestinationPath)
				}
				storedBefore := *f.wt
				admission, err := f.manager.AdmitRecovery(context.Background(), f.request)
				require.NoError(t, err)
				require.Nil(t, admission)
				require.Equal(t, before, captureLegacyReuseContent(t, f.wt.Path))
				require.Equal(t, storedBefore, *f.store.worktrees[storedBefore.ID])
				require.Nil(t, f.store.claim)
				if !destinationExists {
					require.NoDirExists(t, f.proof.ExpectedDestinationPath)
				}
			})
		}
	}
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.6
func TestManagerAdmitRecoveryRejectsInvalidLegacyReuse(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, *legacyReuseFixture)
	}{
		{"wrong origin", func(t *testing.T, f *legacyReuseFixture) {
			runGit(t, f.wt.RepositoryPath, "remote", "set-url", "origin", "https://github.com/other/widget.git")
		}},
		{"missing registered destination", func(_ *testing.T, f *legacyReuseFixture) {
			f.request.Slots[0].RepositoryPath = f.proof.ExpectedDestinationPath
		}},
		{"unrecognized source", func(_ *testing.T, f *legacyReuseFixture) {
			f.proof.ExpectedSourcePath = ""
			f.proof.LegacyOwnerNameSourcePath = ""
		}},
		{"foreign workspace", func(t *testing.T, f *legacyReuseFixture) {
			foreign := filepath.Join(f.proof.ManagedRoot, "workspaces", "foreign", "github", "acme", "widget")
			require.NoError(t, os.MkdirAll(filepath.Dir(foreign), 0o755))
			runGit(t, f.wt.RepositoryPath, "clone", "--no-hardlinks", f.wt.RepositoryPath, foreign)
			f.request.Slots[0].RepositoryPath = foreign
		}},
		{"different Git registration", func(t *testing.T, f *legacyReuseFixture) {
			source := f.proof.ExpectedSourcePath
			require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
			runGit(t, f.wt.RepositoryPath, "clone", "--no-hardlinks", f.wt.RepositoryPath, source)
			runGit(t, source, "remote", "set-url", "origin", "https://github.com/acme/widget.git")
			f.request.Slots[0].RepositoryPath = source
		}},
		{"wrong branch", func(_ *testing.T, f *legacyReuseFixture) { f.wt.Branch = "feature/not-registered" }},
		{"stale recorded source", func(_ *testing.T, f *legacyReuseFixture) {
			f.proof.RecordedSourcePath = f.proof.ExpectedDestinationPath
			f.proof.RecordedSourceCommonDir = filepath.Join(f.proof.ExpectedDestinationPath, ".git")
		}},
		{"invalid sibling", func(_ *testing.T, f *legacyReuseFixture) {
			sibling := *f.wt
			sibling.ID = "wt-sibling"
			sibling.RepositoryID = "repo-sibling"
			f.store.worktrees[sibling.ID] = &sibling
			slot := f.request.Slots[0]
			slot.WorktreeID = sibling.ID
			slot.RepositoryID = sibling.RepositoryID
			slot.RepositoryPath = f.proof.ExpectedDestinationPath
			f.request.Slots = append(f.request.Slots, slot)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLegacyReuseFixture(t, "github", true, false)
			tc.mutate(t, &f)
			f.wt.RepositoryPath = f.request.Slots[0].RepositoryPath
			before := runGit(t, f.wt.Path, "rev-parse", "HEAD")
			storedBefore := *f.wt
			_, err := f.manager.AdmitRecovery(context.Background(), f.request)
			require.Error(t, err)
			require.Equal(t, before, runGit(t, f.wt.Path, "rev-parse", "HEAD"))
			require.Equal(t, storedBefore, *f.store.worktrees[storedBefore.ID])
			require.Nil(t, f.store.claim)
			require.NoDirExists(t, f.proof.ExpectedDestinationPath)
		})
	}
}

func TestRegisteredLegacyCloneInspectionCancellation(t *testing.T) {
	f := newLegacyReuseFixture(t, "github", true, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f.manager.inspectRegisteredLegacyClone(ctx, f.wt.TaskID, f.wt, f.proof)
	require.ErrorIs(t, err, context.Canceled)
}

func TestManagerAdmitRecoveryReusesLegacyCloneWithFilesystemCaseAlias(t *testing.T) {
	f := newLegacyReuseFixture(t, "github", true, false)
	alias := filepath.Join(filepath.Dir(f.wt.RepositoryPath), "WIDGET")
	if !sameDirectoryIdentity(alias, f.wt.RepositoryPath) {
		t.Skip("filesystem is case sensitive")
	}
	f.request.Slots[0].RepositoryPath = alias
	f.wt.RepositoryPath = alias
	admission, err := f.manager.AdmitRecovery(context.Background(), f.request)
	require.NoError(t, err)
	require.Nil(t, admission)
}

func TestManagerAdmitRecoveryReusesLegacyCloneThroughRegisteredSymlink(t *testing.T) {
	f := newLegacyReuseFixture(t, "github", true, false)
	alias := filepath.Join(f.proof.ManagedRoot, "registered-alias")
	require.NoError(t, os.Symlink(f.wt.RepositoryPath, alias))
	f.request.Slots[0].RepositoryPath = alias
	f.wt.RepositoryPath = alias
	admission, err := f.manager.AdmitRecovery(context.Background(), f.request)
	require.NoError(t, err)
	require.Nil(t, admission)
}

func TestManagerAdmitRecoveryPreservesDetachedLegacyMainCheckout(t *testing.T) {
	for _, ownerLayout := range []bool{false, true} {
		t.Run(fmt.Sprintf("owner-layout=%t", ownerLayout), func(t *testing.T) {
			f := newLegacyReuseFixture(t, "github", ownerLayout, true)
			runGit(t, f.wt.Path, "checkout", "--detach")
			runGit(t, f.wt.Path, "commit", "--allow-empty", "-m", "detached work")
			head := runGit(t, f.wt.Path, "rev-parse", "HEAD")
			require.NotEqual(t, runGit(t, f.wt.Path, "rev-parse", f.wt.Branch), head)
			storedBefore := *f.wt
			admission, err := f.manager.AdmitRecovery(context.Background(), f.request)
			require.NoError(t, err)
			require.Nil(t, admission)
			require.Equal(t, head, runGit(t, f.wt.Path, "rev-parse", "HEAD"))
			require.Equal(t, "HEAD", strings.TrimSpace(runGit(t, f.wt.Path, "rev-parse", "--abbrev-ref", "HEAD")))
			require.Equal(t, storedBefore, *f.store.worktrees[storedBefore.ID])
			require.NoDirExists(t, f.proof.ExpectedDestinationPath)
			require.Nil(t, f.store.claim)
		})
	}
}
