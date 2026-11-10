package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestContainerCreateSubmitsStorageBounds(t *testing.T) {
	cases := []struct {
		name        string
		interactive bool
		target      string
	}{
		{name: "normal defaults"},
		{name: "interactive defaults", interactive: true},
		{name: "normal explicit mount", target: "/tmp/"},
		{name: "interactive explicit mount", interactive: true, target: "/tmp/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan *container.HostConfig, 1)
			c := newArchiveTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead && r.URL.Path == "/_ping" {
					w.WriteHeader(http.StatusOK)
					return
				}
				if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/containers/create") {
					http.NotFound(w, r)
					return
				}
				var body struct {
					HostConfig *container.HostConfig
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				requests <- body.HostConfig
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"Id":"storage-container"}`))
			})
			cfg := ContainerConfig{Image: "test-image", Memory: 512 * 1024 * 1024}
			if tc.target != "" {
				cfg.Mounts = []MountConfig{{Source: "/host/task-tmp", Target: tc.target, ReadOnly: true}}
			}
			create := (*Client).CreateContainer
			if tc.interactive {
				create = (*Client).CreateContainerInteractive
			}
			id, err := create(c, context.Background(), cfg)
			require.NoError(t, err)
			require.Equal(t, "storage-container", id)
			var got *container.HostConfig
			select {
			case got = <-requests:
			default:
				t.Fatal("container creation did not submit a request")
			}
			assertCreatedStorageBounds(t, got, cfg)
		})
	}
}

func assertCreatedStorageBounds(t *testing.T, got *container.HostConfig, cfg ContainerConfig) {
	t.Helper()
	require.NotNil(t, got)
	require.Equal(t, container.LogConfig{
		Type: "json-file", Config: map[string]string{"max-size": "100m", "max-file": "3"},
	}, got.LogConfig)
	require.Equal(t, cfg.Memory, got.Memory)
	if len(cfg.Mounts) == 0 {
		require.Equal(t, "rw,exec,nosuid,nodev,size=2147483648,mode=1777", got.Tmpfs["/tmp"])
		return
	}
	require.NotContains(t, got.Tmpfs, "/tmp")
	require.Len(t, got.Mounts, 1)
	require.Equal(t, cfg.Mounts[0].Source, got.Mounts[0].Source)
	require.Equal(t, cfg.Mounts[0].Target, got.Mounts[0].Target)
	require.True(t, got.Mounts[0].ReadOnly)
}
