package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/stretchr/testify/require"
)

func TestShouldPersistMetadataKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "exact match sprite_name", key: "sprite_name", want: true},
		{name: "exact match is_remote", key: MetadataKeyIsRemote, want: true},
		{name: "exact match cleanup_script", key: MetadataKeyCleanupScript, want: true},
		{name: "exact match executor_profile_id", key: "executor_profile_id", want: true},
		{name: "exact match image_tag_override", key: MetadataKeyImageTagOverride, want: true},
		{name: "exact match container_id", key: MetadataKeyContainerID, want: true},
		{name: "prefix env_secret_id_", key: "env_secret_id_SPRITES_API_TOKEN", want: true},
		{name: "prefix env_secret_id_ another key", key: "env_secret_id_OPENAI_KEY", want: true},
		{name: "not persistent task_description", key: "task_description", want: false},
		{name: "not persistent session_id", key: "session_id", want: false},
		{name: "not persistent empty", key: "", want: false},
		{name: "not persistent arbitrary key", key: "some_random_key", want: false},
		{name: "partial prefix match (no underscore)", key: "env_secret_id", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldPersistMetadataKey(tt.key)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestFilterPersistentMetadata(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		require.Nil(t, FilterPersistentMetadata(nil))
	})

	t.Run("empty map returns nil", func(t *testing.T) {
		require.Nil(t, FilterPersistentMetadata(map[string]interface{}{}))
	})

	t.Run("no persistent keys returns nil", func(t *testing.T) {
		src := map[string]interface{}{
			"task_description": "do something",
			"session_id":       "abc",
		}
		require.Nil(t, FilterPersistentMetadata(src))
	})

	t.Run("filters to persistent keys only", func(t *testing.T) {
		src := map[string]interface{}{
			"sprite_name":                     "kandev-abc",
			"task_description":                "should be dropped",
			"env_secret_id_SPRITES_API_TOKEN": "secret-123",
			MetadataKeyIsRemote:               true,
		}
		got := FilterPersistentMetadata(src)
		require.NotNil(t, got)
		require.Equal(t, "kandev-abc", got["sprite_name"])
		require.Equal(t, "secret-123", got["env_secret_id_SPRITES_API_TOKEN"])
		require.Equal(t, true, got[MetadataKeyIsRemote])
		require.NotContains(t, got, "task_description")
	})
}

func TestToAgentExecutionRecordsHistoryForWorkspaceRebindFallback(t *testing.T) {
	instance := &ExecutorInstance{InstanceID: "execution"}
	execution := instance.ToAgentExecution(&ExecutorCreateRequest{
		AgentConfig: agents.NewOpenCodeACP(),
	})

	require.True(t, execution.historyEnabled)
}

func TestToAgentExecutionCapturesDefensiveRuntimeEnvironment(t *testing.T) {
	reqEnv := map[string]string{
		"KANDEV_GITHUB_CREDENTIAL_BROKER_URL": "http://127.0.0.1:9876",
		"PATH":                                "/tmp/kandev-shim:/usr/bin",
	}
	execution := (&ExecutorInstance{InstanceID: "execution"}).ToAgentExecution(&ExecutorCreateRequest{Env: reqEnv})

	reqEnv["PATH"] = "/usr/bin"
	got := execution.RuntimeEnvironment()
	require.Equal(t, "/tmp/kandev-shim:/usr/bin", got["PATH"])

	got["PATH"] = "/mutated"
	require.Equal(t, "/tmp/kandev-shim:/usr/bin", execution.RuntimeEnvironment()["PATH"])
}

func TestToAgentExecutionCapturesRunID(t *testing.T) {
	execution := (&ExecutorInstance{InstanceID: "execution"}).ToAgentExecution(&ExecutorCreateRequest{
		Env: map[string]string{"KANDEV_RUN_ID": "run-1"},
	})
	require.Equal(t, "run-1", execution.RunID)
}

func TestToAgentExecutionCapturesDefensiveMetadata(t *testing.T) {
	requestMetadata := map[string]interface{}{"request": "original"}
	execution := (&ExecutorInstance{
		InstanceID: "execution",
		Metadata:   map[string]interface{}{"runtime": "value"},
	}).ToAgentExecution(&ExecutorCreateRequest{Metadata: requestMetadata})

	require.Equal(t, map[string]interface{}{
		"request": "original",
		"runtime": "value",
	}, execution.MetadataSnapshot())
	execution.setMetadataValue("persisted", "secret-ref")
	require.Equal(t, map[string]interface{}{"request": "original"}, requestMetadata)
}
