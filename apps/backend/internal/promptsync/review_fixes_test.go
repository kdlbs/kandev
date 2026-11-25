package promptsync

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/gitlab"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/reposync"
)

type recordingGitLab struct {
	*fakeGitLab
	workspaces []string
	tooLarge   map[string]bool
}

func (r *recordingGitLab) ListRepoTreeForWorkspace(ctx context.Context, workspaceID, project, path, ref string) ([]gitlab.RepoTreeEntry, error) {
	r.workspaces = append(r.workspaces, workspaceID)
	return r.fakeGitLab.ListRepoTreeForWorkspace(ctx, workspaceID, project, path, ref)
}

func (r *recordingGitLab) GetRepoFileContentForWorkspace(ctx context.Context, workspaceID, project, path, ref string) ([]byte, error) {
	for name := range r.tooLarge {
		if path == DefaultPath+"/"+name {
			return nil, fmt.Errorf("file exceeds the limit: %w", gitlab.ErrRepoFileTooLarge)
		}
	}
	return r.fakeGitLab.GetRepoFileContentForWorkspace(ctx, workspaceID, project, path, ref)
}

func withRecordingGitLab(f *syncFixture) *recordingGitLab {
	rec := &recordingGitLab{fakeGitLab: f.gitlab, tooLarge: map[string]bool{}}
	f.svc.gitlab = rec
	return rec
}

func TestSyncUsesTheConfigurationItAuthorized(t *testing.T) {
	f := newSyncFixture(t)
	rec := withRecordingGitLab(f)
	f.gitlab.files["a.md"] = "A"
	f.svc.SetWorkspaceAuthorizer(func(ctx context.Context, id string) error {
		if id != "ws-1" {
			return fmt.Errorf("denied %s", id)
		}
		// A replacement lands between the access check and the read.
		req := &SetConfigRequest{WorkspaceID: "foreign", Provider: reposync.ProviderGitLab, ProjectPath: "x/y"}
		require.NoError(t, req.Normalize())
		_, err := f.svc.store.Upsert(ctx, req)
		return err
	})
	_, err := f.svc.Sync(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"ws-1"}, rec.workspaces, "reads must use the authorized workspace")
}

func TestSyncOversizedFileFreezesOnlyThatPrompt(t *testing.T) {
	f := newSyncFixture(t)
	rec := withRecordingGitLab(f)
	ctx := context.Background()
	f.gitlab.files = map[string]string{"big.md": "v1", "small.md": "v1"}
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)

	f.gitlab.files = map[string]string{"big.md": "v2", "small.md": "v2"}
	rec.tooLarge["big.md"] = true
	result, err := f.svc.Sync(ctx)
	require.NoError(t, err, "one oversized file must not fail the run")
	assert.Equal(t, []string{"small"}, result.Updated)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "big.md")
	big, err := f.prompts.GetPromptByName(ctx, "big")
	require.NoError(t, err)
	assert.Equal(t, "v1", big.Content)
}

func TestSyncWarnsAboutMissingReferencesInKeptPrompts(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()
	f.gitlab.files = map[string]string{"a.md": "Uses @never-defined", "b.md": "B"}
	_, err := f.svc.Sync(ctx)
	require.NoError(t, err)

	f.gitlab.files = map[string]string{}
	f.refs.texts = []promptservice.PromptReferenceText{{Owner: "wf-1", Text: "Run @a"}}
	result, err := f.svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"b"}, result.Deleted)
	joined := fmt.Sprint(result.Warnings)
	assert.Contains(t, joined, `prompt "a" was removed`)
	assert.Contains(t, joined, `prompt "a" references @never-defined`, "a kept prompt is still checked")
}
