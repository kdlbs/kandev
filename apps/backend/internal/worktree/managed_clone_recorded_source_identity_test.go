package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.6
func TestRecordedManagedCloneSourceIdentity(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	common := filepath.Join(source, ".git")
	other := filepath.Join(t.TempDir(), "other")
	require.NoError(t, os.MkdirAll(common, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(other, ".git"), 0o755))
	missing := filepath.Join(t.TempDir(), "missing")
	for _, tc := range []struct {
		name, path, common string
		valid              bool
	}{
		{"legacy", "", "", true},
		{"matching", source, common, true},
		{"missing source field", "", common, false},
		{"missing common field", source, "", false},
		{"missing source", missing, common, false},
		{"missing common", source, missing, false},
		{"different source", other, common, false},
		{"different common", source, filepath.Join(other, ".git"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyRecordedManagedCloneSource(source, common, &ManagedCloneRelocationProof{RecordedSourcePath: tc.path, RecordedSourceCommonDir: tc.common})
			require.Equal(t, tc.valid, err == nil)
		})
	}
	t.Run("distinct case sensitive directory", func(t *testing.T) {
		alias := filepath.Join(filepath.Dir(source), "SOURCE")
		if sameDirectoryIdentity(source, alias) {
			t.Skip("requires case-sensitive filesystem")
		}
		require.NoError(t, os.MkdirAll(filepath.Join(alias, ".git"), 0o755))
		require.Error(t, verifyRecordedManagedCloneSource(source, common, &ManagedCloneRelocationProof{RecordedSourcePath: alias, RecordedSourceCommonDir: filepath.Join(alias, ".git")}))
	})
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.5
func TestManagerAdmitRecoveryReusesRecordedSourceCaseAlias(t *testing.T) {
	for _, provider := range []string{"github", "gitlab"} {
		for _, ownerLayout := range []bool{false, true} {
			for _, mainCheckout := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/owner=%t/main=%t", provider, ownerLayout, mainCheckout), func(t *testing.T) {
					f := newLegacyReuseFixture(t, provider, ownerLayout, mainCheckout)
					alias := filepath.Join(filepath.Dir(f.wt.RepositoryPath), "WIDGET")
					if !sameDirectoryIdentity(alias, f.wt.RepositoryPath) {
						t.Skip("requires case-insensitive filesystem")
					}
					require.NoError(t, os.WriteFile(filepath.Join(f.wt.Path, "staged.txt"), []byte("staged\n"), 0o600))
					runGit(t, f.wt.Path, "add", "staged.txt")
					require.NoError(t, os.WriteFile(filepath.Join(f.wt.Path, "staged.txt"), []byte("edited\n"), 0o600))
					head := runGit(t, f.wt.Path, "rev-parse", "HEAD")
					indexPath := strings.TrimSpace(runGit(t, f.wt.Path, "rev-parse", "--path-format=absolute", "--git-path", "index"))
					index, err := os.ReadFile(indexPath)
					require.NoError(t, err)
					for _, fields := range []string{"source", "common", "both"} {
						t.Run(fields, func(t *testing.T) {
							f.proof.RecordedSourcePath = f.wt.RepositoryPath
							f.proof.RecordedSourceCommonDir = filepath.Join(f.wt.RepositoryPath, ".git")
							if fields != "common" {
								f.proof.RecordedSourcePath = alias
							}
							if fields != "source" {
								f.proof.RecordedSourceCommonDir = filepath.Join(alias, ".git")
							}
							stored := *f.store.worktrees[f.wt.ID]
							admission, err := f.manager.AdmitRecovery(context.Background(), f.request)
							require.NoError(t, err)
							require.Nil(t, admission)
							require.Equal(t, stored, *f.store.worktrees[f.wt.ID])
							require.Equal(t, head, runGit(t, f.wt.Path, "rev-parse", "HEAD"))
							after, err := os.ReadFile(indexPath)
							require.NoError(t, err)
							require.Equal(t, index, after)
							data, err := os.ReadFile(filepath.Join(f.wt.Path, "staged.txt"))
							require.NoError(t, err)
							require.Equal(t, "edited\n", string(data))
							require.NoDirExists(t, f.proof.ExpectedDestinationPath)
							require.Nil(t, f.store.claim)
						})
					}
				})
			}
		}
	}
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.6
func TestManagerAdmitRecoveryRejectsRecordedSourceAliasWithInvalidSibling(t *testing.T) {
	f := newLegacyReuseFixture(t, "github", true, false)
	alias := filepath.Join(filepath.Dir(f.wt.RepositoryPath), "WIDGET")
	if !sameDirectoryIdentity(alias, f.wt.RepositoryPath) {
		t.Skip("requires case-insensitive filesystem")
	}
	f.proof.RecordedSourcePath = alias
	f.proof.RecordedSourceCommonDir = filepath.Join(alias, ".git")
	sibling := *f.wt
	sibling.ID = "invalid-sibling"
	sibling.RepositoryID = "other-repo"
	f.store.worktrees[sibling.ID] = &sibling
	slot := f.request.Slots[0]
	slot.WorktreeID = sibling.ID
	slot.RepositoryID = sibling.RepositoryID
	slot.RepositoryPath = f.proof.ExpectedDestinationPath
	f.request.Slots = append(f.request.Slots, slot)
	before := *f.wt
	_, err := f.manager.AdmitRecovery(context.Background(), f.request)
	require.Error(t, err)
	require.Equal(t, before, *f.store.worktrees[f.wt.ID])
	require.Nil(t, f.store.claim)
	require.NoDirExists(t, f.proof.ExpectedDestinationPath)
}
