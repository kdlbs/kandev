package reposync

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

var errTestInvalid = errors.New("invalid test source")

func TestTargetNormalizeUsesCallerDefaultPathAndSentinel(t *testing.T) {
	target := Target{RepoOwner: " acme ", RepoName: "eng", Path: "/"}
	require.NoError(t, target.Normalize(".kandev/prompts", errTestInvalid))
	assert.Equal(t, ProviderGitHub, target.Provider)
	assert.Equal(t, "acme", target.RepoOwner)
	assert.Equal(t, ".kandev/prompts", target.Path)
	assert.Equal(t, DefaultBranch, target.Branch)
	assert.Equal(t, DefaultIntervalSeconds, target.IntervalSeconds)

	bad := Target{Provider: ProviderGitLab, ProjectPath: "single"}
	assert.ErrorIs(t, bad.Normalize(".kandev/prompts", errTestInvalid), errTestInvalid)
}

type fakeGitHub struct {
	GitHubClientProvider
	entries []github.RepoContentEntry
	files   map[string][]byte
	listed  string
}

func (f *fakeGitHub) ListRepoDirectoryForWorkspace(_ context.Context, _, _, _, path, _ string) ([]github.RepoContentEntry, error) {
	f.listed = path
	return f.entries, nil
}

func (f *fakeGitHub) GetRepoFileContentForWorkspace(_ context.Context, _, _, _, path, _ string) ([]byte, error) {
	return f.files[path], nil
}

func TestReaderListsAndReadsGitHub(t *testing.T) {
	gh := &fakeGitHub{
		entries: []github.RepoContentEntry{{Name: "a.md", Path: "p/a.md", Type: "file"}, {Name: "sub", Path: "p/sub", Type: "dir"}},
		files:   map[string][]byte{"p/a.md": []byte("hello")},
	}
	reader := Reader{GitHub: gh}
	src := Source{Provider: ProviderGitHub, RepoOwner: "o", RepoName: "r", Branch: "main", Path: "p"}
	entries, err := reader.List(context.Background(), src)
	require.NoError(t, err)
	assert.Equal(t, "p", gh.listed)
	assert.Equal(t, []Entry{{Name: "a.md", Path: "p/a.md", IsFile: true}, {Name: "sub", Path: "p/sub", IsDir: true}}, entries)
	get, err := reader.Getter(src)
	require.NoError(t, err)
	content, err := get(context.Background(), "p/a.md")
	require.NoError(t, err)
	assert.Equal(t, "hello", string(content))
}

func TestReaderWithoutClientReportsNotConfigured(t *testing.T) {
	reader := Reader{}
	_, err := reader.List(context.Background(), Source{Provider: ProviderGitHub})
	assert.ErrorIs(t, err, ErrGitHubClientNotConfigured)
	_, err = reader.List(context.Background(), Source{Provider: ProviderGitLab})
	assert.ErrorIs(t, err, ErrGitLabClientNotConfigured)
	_, err = reader.Getter(Source{Provider: ProviderGitLab})
	assert.ErrorIs(t, err, ErrGitLabClientNotConfigured)
}

type fakeGitLab struct {
	GitLabClientProvider
	entries []gitlab.RepoTreeEntry
}

func (f *fakeGitLab) ListRepoTreeForWorkspace(context.Context, string, string, string, string) ([]gitlab.RepoTreeEntry, error) {
	return f.entries, nil
}

func TestReaderMapsGitLabTreeEntries(t *testing.T) {
	reader := Reader{GitLab: &fakeGitLab{entries: []gitlab.RepoTreeEntry{
		{Name: "a.md", Path: "p/a.md", Type: gitlab.TreeEntryTypeBlob},
		{Name: "sub", Path: "p/sub", Type: gitlab.TreeEntryTypeTree},
	}}}
	entries, err := reader.List(context.Background(), Source{Provider: ProviderGitLab, ProjectPath: "g/p", Path: "p"})
	require.NoError(t, err)
	assert.Equal(t, []Entry{{Name: "a.md", Path: "p/a.md", IsFile: true}, {Name: "sub", Path: "p/sub", IsDir: true}}, entries)
}
