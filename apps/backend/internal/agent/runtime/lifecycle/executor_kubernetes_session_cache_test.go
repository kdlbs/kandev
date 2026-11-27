package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKubernetesSessionEnvironmentDefaultsCachesToRuntimeVolume(t *testing.T) {
	req := &ExecutorCreateRequest{
		TaskID:    "task-1",
		SessionID: "session-1",
		Metadata:  map[string]interface{}{metadataKubernetesTaskOwned: true},
	}

	env := kubernetesSessionEnvironment(req)

	require.Equal(t, "/opt/kandev/cache/npm", env["NPM_CONFIG_CACHE"])
	require.Equal(t, "/opt/kandev/cache/npm", env["npm_config_cache"])
	require.Equal(t, "/opt/kandev/cache/xdg", env["XDG_CACHE_HOME"])
	require.Regexp(t, `^/run/kandev/sessions/[a-f0-9]{32}/home$`, env["HOME"])
}

func TestKubernetesSessionEnvironmentDoesNotDefaultCachesForLegacySessions(t *testing.T) {
	env := kubernetesSessionEnvironment(&ExecutorCreateRequest{
		TaskID:    "task-1",
		SessionID: "session-1",
	})

	require.NotContains(t, env, "NPM_CONFIG_CACHE")
	require.NotContains(t, env, "npm_config_cache")
	require.NotContains(t, env, "XDG_CACHE_HOME")
}

func TestKubernetesSessionEnvironmentKeepsProfileCacheLocations(t *testing.T) {
	tests := []struct {
		name       string
		profileEnv map[string]string
		want       map[string]string
		absent     []string
	}{
		{
			name:       "uppercase npm cache",
			profileEnv: map[string]string{"NPM_CONFIG_CACHE": "/profile/npm"},
			want: map[string]string{
				"NPM_CONFIG_CACHE": "/profile/npm",
				"XDG_CACHE_HOME":   "/opt/kandev/cache/xdg",
			},
			absent: []string{"npm_config_cache"},
		},
		{
			name:       "lowercase npm cache",
			profileEnv: map[string]string{"npm_config_cache": "/profile/npm-lower"},
			want: map[string]string{
				"npm_config_cache": "/profile/npm-lower",
				"XDG_CACHE_HOME":   "/opt/kandev/cache/xdg",
			},
			absent: []string{"NPM_CONFIG_CACHE"},
		},
		{
			name:       "mixed-case npm cache",
			profileEnv: map[string]string{"Npm_Config_Cache": "/profile/npm-mixed"},
			want:       map[string]string{"Npm_Config_Cache": "/profile/npm-mixed"},
			absent:     []string{"NPM_CONFIG_CACHE", "npm_config_cache"},
		},
		{
			name:       "empty npm cache",
			profileEnv: map[string]string{"NPM_CONFIG_CACHE": ""},
			want: map[string]string{
				"NPM_CONFIG_CACHE": "/opt/kandev/cache/npm",
				"npm_config_cache": "/opt/kandev/cache/npm",
				"XDG_CACHE_HOME":   "/opt/kandev/cache/xdg",
			},
		},
		{
			name:       "empty mixed-case npm cache",
			profileEnv: map[string]string{"Npm_Config_Cache": ""},
			want: map[string]string{
				"NPM_CONFIG_CACHE": "/opt/kandev/cache/npm",
				"Npm_Config_Cache": "/opt/kandev/cache/npm",
				"npm_config_cache": "/opt/kandev/cache/npm",
				"XDG_CACHE_HOME":   "/opt/kandev/cache/xdg",
			},
		},
		{
			name:       "XDG cache home",
			profileEnv: map[string]string{"XDG_CACHE_HOME": "/profile/xdg"},
			want: map[string]string{
				"NPM_CONFIG_CACHE": "/opt/kandev/cache/npm",
				"npm_config_cache": "/opt/kandev/cache/npm",
				"XDG_CACHE_HOME":   "/profile/xdg",
			},
		},
		{
			name:       "empty XDG cache home",
			profileEnv: map[string]string{"XDG_CACHE_HOME": ""},
			want: map[string]string{
				"NPM_CONFIG_CACHE": "/opt/kandev/cache/npm",
				"npm_config_cache": "/opt/kandev/cache/npm",
				"XDG_CACHE_HOME":   "/opt/kandev/cache/xdg",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ExecutorCreateRequest{
				TaskID:    "task-1",
				SessionID: "session-1",
				Env:       tt.profileEnv,
				Metadata:  map[string]interface{}{metadataKubernetesTaskOwned: true},
			}

			env := kubernetesSessionEnvironment(req)

			for key, want := range tt.want {
				require.Equal(t, want, env[key], key)
			}
			for _, key := range tt.absent {
				_, ok := env[key]
				require.False(t, ok, key)
			}
		})
	}
}
