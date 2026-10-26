package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func fixedHooksProvider(hooks map[string]bool, err error) GitHooksProvider {
	return func(context.Context, string) (map[string]bool, error) { return hooks, err }
}

func TestSeedExecutionGitHooks(t *testing.T) {
	local := string(models.ExecutorTypeWorktree)
	cases := []struct {
		name     string
		provider GitHooksProvider
		taskID   string
		executor string
		existing interface{}
		want     map[string]bool
	}{
		{"enabled repository", fixedHooksProvider(map[string]bool{"": true}, nil), "t", local, nil, map[string]bool{"": true}},
		{"empty non-nil map is off", fixedHooksProvider(map[string]bool{}, nil), "t", local, nil, nil},
		{"nil map is off", fixedHooksProvider(nil, nil), "t", local, nil, nil},
		{"provider error discards a partial map", fixedHooksProvider(map[string]bool{"": true}, errors.New("boom")), "t", local, nil, nil},
		{"unwired provider is off", nil, "t", local, nil, nil},
		{"empty task id skips the provider", fixedHooksProvider(map[string]bool{"": true}, nil), "", local, nil, nil},
		{"kubernetes gets no key", fixedHooksProvider(map[string]bool{"": true}, nil), "t", string(models.ExecutorTypeKubernetes), nil, nil},
		{"plugin remote gets no key", fixedHooksProvider(map[string]bool{"": true}, nil), "t", string(models.ExecutorTypePluginRemote), nil, nil},
		{"stale key from a retried request is removed", fixedHooksProvider(map[string]bool{}, nil), "t", local, map[string]bool{"old": true}, nil},
		{"stale key is replaced by the stored value", fixedHooksProvider(map[string]bool{"new": true}, nil), "t", local, map[string]bool{"old": true}, map[string]bool{"new": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := newTestManager(t)
			mgr.SetGitHooksProvider(tc.provider)
			metadata := map[string]interface{}{}
			if tc.existing != nil {
				metadata[MetadataKeyGitHooks] = tc.existing
			}
			mgr.seedExecutionGitHooks(context.Background(), tc.taskID, tc.executor, metadata)
			assert.Equal(t, tc.want, gitHooksFromMetadata(metadata))
			if tc.want == nil {
				assert.NotContains(t, metadata, MetadataKeyGitHooks)
			}
		})
	}
}

func TestSeedExecutionGitHooksCopiesTheProviderMap(t *testing.T) {
	mgr := newTestManager(t)
	stored := map[string]bool{"": true}
	mgr.SetGitHooksProvider(fixedHooksProvider(stored, nil))
	metadata := map[string]interface{}{}
	mgr.seedExecutionGitHooks(context.Background(), "t", string(models.ExecutorTypeWorktree), metadata)
	stored[""] = false
	assert.Equal(t, map[string]bool{"": true}, gitHooksFromMetadata(metadata))
	got := gitHooksFromMetadata(metadata)
	got[""] = false
	assert.Equal(t, map[string]bool{"": true}, gitHooksFromMetadata(metadata))
}

func TestSeedExecutionGitHooksReadsAtSeedTime(t *testing.T) {
	mgr := newTestManager(t)
	value := map[string]bool{}
	mgr.SetGitHooksProvider(func(context.Context, string) (map[string]bool, error) { return value, nil })
	metadata := map[string]interface{}{}
	value = map[string]bool{"": true} // changed after the request was being built, before the seed
	mgr.seedExecutionGitHooks(context.Background(), "t", string(models.ExecutorTypeWorktree), metadata)
	assert.Equal(t, map[string]bool{"": true}, gitHooksFromMetadata(metadata))
}

func TestPrepareExecutionCreateRequestSeedsGitHooksFromProvider(t *testing.T) {
	mgr := newTestManager(t)
	mgr.SetGitHooksProvider(fixedHooksProvider(map[string]bool{"": true}, nil))
	info := &WorkspaceInfo{
		TaskID: "task-1", SessionID: "session-2", WorkspacePath: "/workspace", AgentID: "auggie",
		ExecutorType: string(models.ExecutorTypeLocalDocker),
		Metadata:     map[string]interface{}{MetadataKeyGitHooks: map[string]bool{"other": true}},
	}
	prepared, err := mgr.prepareExecutionCreateRequest(context.Background(), "task-1", info, "execution-2")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"": true}, gitHooksFromMetadata(prepared.request.Metadata))

	info.ExecutorType = string(models.ExecutorTypeKubernetes)
	prepared, err = mgr.prepareExecutionCreateRequest(context.Background(), "task-1", info, "execution-3")
	require.NoError(t, err)
	assert.NotContains(t, prepared.request.Metadata, MetadataKeyGitHooks)
}

func TestRequestBuildersCopyTheSeededGitHooks(t *testing.T) {
	seeded := &ExecutorCreateRequest{Metadata: map[string]interface{}{MetadataKeyGitHooks: map[string]bool{"": true}}}
	reconnect := buildReconnectCreateInstanceRequest(seeded, "instance-1")
	assert.Equal(t, map[string]bool{"": true}, reconnect.GitHooks)
	seeded.Metadata[MetadataKeyGitHooks].(map[string]bool)[""] = false
	assert.Equal(t, map[string]bool{"": true}, reconnect.GitHooks, "the wire request owns its own copy")

	unseeded := &ExecutorCreateRequest{ExecutorType: string(models.ExecutorTypeKubernetes), Metadata: map[string]interface{}{}}
	assert.Nil(t, buildReconnectCreateInstanceRequest(unseeded, "instance-2").GitHooks)

	config, err := buildDockerContainerConfig(&ExecutorCreateRequest{
		InstanceID: "instance-3", WorkspacePath: "/workspace",
		Metadata: map[string]interface{}{MetadataKeyGitHooks: map[string]bool{"app": true}},
	}, string(models.ExecutorTypeLocalDocker))
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"app": true}, config.GitHooks)
}

func TestExecutorRequestBuildersCarrySeededGitHooksToTheWire(t *testing.T) {
	newReq := func() *ExecutorCreateRequest {
		return &ExecutorCreateRequest{
			InstanceID: "instance-wire", WorkspacePath: "/workspace",
			Metadata: map[string]interface{}{MetadataKeyGitHooks: map[string]bool{"": true}},
		}
	}
	want := map[string]bool{"": true}

	standalone := buildStandaloneCreateInstanceRequest(newReq(), nil, "", false, false, false, false, nil)
	assert.Equal(t, want, standalone.GitHooks, "standalone")
	assert.Equal(t, want, buildSSHCreateInstanceRequest(newReq(), "/workspace", "agentctl").GitHooks, "ssh")
	assert.Equal(t, want, agentctlInstanceRequest(newReq(), "/workspace").GitHooks, "sprites")

	unseeded := &ExecutorCreateRequest{InstanceID: "instance-none", Metadata: map[string]interface{}{}}
	assert.Nil(t, buildStandaloneCreateInstanceRequest(unseeded, nil, "", false, false, false, false, nil).GitHooks)
	assert.Nil(t, buildSSHCreateInstanceRequest(unseeded, "/workspace", "agentctl").GitHooks)
	assert.Nil(t, agentctlInstanceRequest(unseeded, "/workspace").GitHooks)
}
