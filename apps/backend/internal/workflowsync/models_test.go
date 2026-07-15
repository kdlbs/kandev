package workflowsync

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetConfigRequestNormalize_AppliesDefaults(t *testing.T) {
	req := &SetConfigRequest{RepoOwner: " acme ", RepoName: "flows"}
	require.NoError(t, req.Normalize())
	assert.Equal(t, "acme", req.RepoOwner)
	assert.Equal(t, DefaultBranch, req.Branch)
	assert.Equal(t, DefaultPath, req.Path)
	assert.Equal(t, DefaultIntervalSeconds, req.IntervalSeconds)
}

func TestSetConfigRequestNormalize_TrimsPathSlashes(t *testing.T) {
	req := &SetConfigRequest{RepoOwner: "acme", RepoName: "flows", Path: "/workflows/prod/"}
	require.NoError(t, req.Normalize())
	assert.Equal(t, "workflows/prod", req.Path)
}

func TestSetConfigRequestNormalize_Rejections(t *testing.T) {
	cases := map[string]*SetConfigRequest{
		"missing owner":      {RepoName: "flows"},
		"owner with slash":   {RepoOwner: "acme/evil", RepoName: "flows"},
		"missing repo":       {RepoOwner: "acme"},
		"repo with space":    {RepoOwner: "acme", RepoName: "my flows"},
		"path traversal":     {RepoOwner: "acme", RepoName: "flows", Path: "a/../../etc"},
		"interval below min": {RepoOwner: "acme", RepoName: "flows", IntervalSeconds: 30},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			err := req.Normalize()
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInvalidConfig))
		})
	}
}

func TestSetConfigRequestNormalize_PollEnabledDefaultsTrue(t *testing.T) {
	req := &SetConfigRequest{RepoOwner: "acme", RepoName: "flows"}
	require.NoError(t, req.Normalize())
	require.NotNil(t, req.PollEnabled)
	assert.True(t, *req.PollEnabled)

	disabled := false
	req = &SetConfigRequest{RepoOwner: "acme", RepoName: "flows", PollEnabled: &disabled}
	require.NoError(t, req.Normalize())
	assert.False(t, *req.PollEnabled, "explicit false survives normalization")
}
