package workflowsync

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An omitted provider must keep meaning GitHub so existing API clients and
// already-stored rows behave exactly as before.
func TestNormalize_DefaultsProviderToGitHub(t *testing.T) {
	req := &SetConfigRequest{RepoOwner: "acme", RepoName: "flows"}
	require.NoError(t, req.Normalize())
	assert.Equal(t, ProviderGitHub, req.Provider)
	assert.Empty(t, req.ProjectPath)
}

func TestNormalize_RejectsUnknownProvider(t *testing.T) {
	req := &SetConfigRequest{Provider: "bitbucket", RepoOwner: "acme", RepoName: "flows"}
	err := req.Normalize()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidConfig))
}

func TestNormalize_GitHubRejectsProjectPath(t *testing.T) {
	req := &SetConfigRequest{
		Provider:    ProviderGitHub,
		RepoOwner:   "acme",
		RepoName:    "flows",
		ProjectPath: "acme/flows",
	}
	err := req.Normalize()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidConfig))
}

func TestNormalize_GitLabAcceptsNestedProjectPath(t *testing.T) {
	req := &SetConfigRequest{Provider: ProviderGitLab, ProjectPath: " acme/team/project "}
	require.NoError(t, req.Normalize())
	assert.Equal(t, "acme/team/project", req.ProjectPath)
	assert.Empty(t, req.RepoOwner)
	assert.Empty(t, req.RepoName)
	// Shared defaults still apply regardless of provider.
	assert.Equal(t, DefaultBranch, req.Branch)
	assert.Equal(t, DefaultPath, req.Path)
	assert.Equal(t, DefaultIntervalSeconds, req.IntervalSeconds)
}

func TestNormalize_GitLabRejectsOwnerAndName(t *testing.T) {
	req := &SetConfigRequest{
		Provider:    ProviderGitLab,
		ProjectPath: "acme/project",
		RepoOwner:   "acme",
	}
	err := req.Normalize()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidConfig))
}

func TestNormalize_GitLabProjectPathValidation(t *testing.T) {
	cases := map[string]string{
		"empty":           "",
		"single segment":  "project",
		"leading slash":   "/acme/project",
		"trailing slash":  "acme/project/",
		"empty segment":   "acme//project",
		"contains space":  "acme/my project",
		"dot segment":     "acme/./project",
		"traversal":       "acme/../project",
		"only separators": "//",
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			req := &SetConfigRequest{Provider: ProviderGitLab, ProjectPath: path}
			err := req.Normalize()
			require.Error(t, err, "expected %q to be rejected", path)
			assert.True(t, errors.Is(err, ErrInvalidConfig))
		})
	}
}

// The GitHub branch of validation must be untouched by the provider split.
func TestNormalize_GitHubOwnerNameRulesUnchanged(t *testing.T) {
	for name, req := range map[string]*SetConfigRequest{
		"missing owner":   {RepoOwner: "", RepoName: "flows"},
		"missing name":    {RepoOwner: "acme", RepoName: ""},
		"owner has slash": {RepoOwner: "acme/team", RepoName: "flows"},
		"name has space":  {RepoOwner: "acme", RepoName: "my flows"},
	} {
		t.Run(name, func(t *testing.T) {
			err := req.Normalize()
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidConfig))
		})
	}
}
