package github

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.15
func TestRepositoryProvidersPreservePushPermissionPresence(t *testing.T) {
	for _, tt := range []struct {
		name, permissions string
		known, push       bool
	}{
		{"writer", `{"push":true,"admin":true}`, true, true},
		{"reader", `{"push":false,"admin":true}`, true, false},
		{"missing", `{}`, false, false},
		{"null push", `{"push":null}`, false, false},
		{"null permissions", `null`, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"full_name":"acme/site","owner":{"login":"acme"},"name":"site","fork":true,"parent":{"full_name":"upstream/site","owner":{"login":"upstream"},"name":"site"},"permissions":` + tt.permissions + `}`
			t.Run("CLI", func(t *testing.T) {
				newFakeGH(t, ghResponse{Prefix: "api /repos/acme/site", Stdout: body})
				repository, err := NewGHClient().GetRepository(context.Background(), "acme", "site")
				require.NoError(t, err)
				assertRepositoryAccess(t, repository, tt.known, tt.push)
			})
			t.Run("PAT", func(t *testing.T) {
				client, requests := newRecordingPATServerFunc(t, func(*http.Request) (int, string) { return http.StatusOK, body })
				repository, err := client.GetRepository(context.Background(), "acme", "site")
				require.NoError(t, err)
				require.Len(t, *requests, 1)
				assertRepositoryAccess(t, repository, tt.known, tt.push)
			})
		})
	}
}

func assertRepositoryAccess(t *testing.T, repository *GitHubRepository, known, push bool) {
	t.Helper()
	require.NotNil(t, repository)
	require.Equal(t, known, repository.PushAccessKnown)
	require.Equal(t, push, repository.PushAccess)
	require.Equal(t, "acme/site", repository.FullName)
	require.True(t, repository.Fork)
	require.Equal(t, "upstream/site", repository.ParentFullName)
}

func TestMockRepositoryDetailsReset(t *testing.T) {
	client := NewMockClient()
	seedMergeAccessRepository(client, "acme", true, true)
	client.Reset()
	repository, err := client.GetRepository(context.Background(), "acme", "site")
	require.Error(t, err)
	require.Nil(t, repository)
}
