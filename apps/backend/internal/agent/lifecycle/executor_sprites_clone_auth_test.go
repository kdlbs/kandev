package lifecycle

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteGitHubSSHToHTTPS(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "scp style",
			in:   "git@github.com:kdlbs/agents-protocol-debug.git",
			want: "https://github.com/kdlbs/agents-protocol-debug.git",
		},
		{
			name: "ssh scheme",
			in:   "ssh://git@github.com/kdlbs/agents-protocol-debug.git",
			want: "https://github.com/kdlbs/agents-protocol-debug.git",
		},
		{
			name: "non github",
			in:   "git@gitlab.com:org/repo.git",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, rewriteGitHubSSHToHTTPS(tc.in))
		})
	}
}

func TestInjectTokenIntoURL_RewritesGitHubSSHWhenTokenExists(t *testing.T) {
	exec := &SpritesExecutor{}
	env := map[string]string{"GITHUB_TOKEN": "test-token"}
	got := exec.injectTokenIntoURL("git@github.com:kdlbs/agents-protocol-debug.git", env)
	require.Equal(t, "https://test-token@github.com/kdlbs/agents-protocol-debug.git", got)
}

func TestIsTransientUploadError(t *testing.T) {
	require.True(t, isTransientUploadError(errors.New("request canceled (Client.Timeout exceeded while awaiting headers)")))
	require.True(t, isTransientUploadError(errors.New("connection reset by peer")))
	require.True(t, isTransientUploadError(errors.New("write /usr/local/bin/agentctl: HTTP 502")))
	require.True(t, isTransientUploadError(errors.New("upload failed: status 429")))
	require.False(t, isTransientUploadError(errors.New("permission denied")))
	require.False(t, isTransientUploadError(errors.New("upload failed: HTTP 400")))
}
