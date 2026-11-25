package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

// @covers AC-EXECUTORS-STORAGE-001.4
// @covers AC-EXECUTORS-STORAGE-001.5
// @covers AC-EXECUTORS-STORAGE-001.6
func TestContainerCreateSubmitsStorageBounds(t *testing.T) {
	cases := []struct {
		name        string
		interactive bool
		cfg         ContainerConfig
		wantTmpfs   string
		wantLogs    map[string]string
	}{
		{name: "normal defaults"},
		{name: "interactive defaults", interactive: true},
		{name: "normal explicit mount", cfg: ContainerConfig{Mounts: []MountConfig{{Target: "/tmp/"}}}},
		{name: "interactive explicit mount", interactive: true, cfg: ContainerConfig{Mounts: []MountConfig{{Target: "/tmp/"}}}},
		{
			name:      "normal temporary storage only",
			cfg:       ContainerConfig{TmpfsSizeBytes: 2 * 1024 * 1024 * 1024},
			wantTmpfs: "rw,exec,nosuid,nodev,size=2147483648,mode=1777",
		},
		{
			name:        "interactive partial log options",
			interactive: true,
			cfg:         ContainerConfig{LogMaxSize: "100m"},
			wantLogs:    map[string]string{"max-size": "100m"},
		},
		{
			name:      "normal full limits",
			cfg:       ContainerConfig{TmpfsSizeBytes: 1024, LogMaxSize: "100m", LogMaxFiles: "3"},
			wantTmpfs: "rw,exec,nosuid,nodev,size=1024,mode=1777",
			wantLogs:  map[string]string{"max-size": "100m", "max-file": "3"},
		},
		{
			name:        "interactive tmp mount takes precedence",
			interactive: true,
			cfg: ContainerConfig{
				TmpfsSizeBytes: 2048,
				LogMaxFiles:    "3",
				Mounts:         []MountConfig{{Source: "/host/task-tmp", Target: "/var/../tmp", ReadOnly: true}},
			},
			wantLogs: map[string]string{"max-file": "3"},
		},
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
			cfg := tc.cfg
			cfg.Name = "storage-test"
			cfg.Image = "test-image"
			cfg.Memory = 512 * 1024 * 1024
			if len(cfg.Mounts) > 0 {
				cfg.Mounts[0].Source = "/host/task-tmp"
				cfg.Mounts[0].ReadOnly = true
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
			assertCreatedStorageOverrides(t, got, cfg, tc.wantTmpfs, tc.wantLogs)
		})
	}
}

func TestCreateContainerReturnsUnsupportedLogOptionErrorWithoutRetry(t *testing.T) {
	var createRequests atomic.Int32
	c := newArchiveTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/_ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/containers/create") {
			createRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"unknown log opt 'max-size' for journald log driver"}`))
			return
		}
		http.NotFound(w, r)
	})

	_, err := c.CreateContainer(context.Background(), ContainerConfig{
		Name:       "unsupported-rotation",
		Image:      "test-image",
		LogMaxSize: "100m",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown log opt 'max-size' for journald log driver")
	require.Equal(t, int32(1), createRequests.Load())
}

func assertCreatedStorageOverrides(t *testing.T, got *container.HostConfig, cfg ContainerConfig, wantTmpfs string, wantLogs map[string]string) {
	t.Helper()
	require.NotNil(t, got)
	require.Equal(t, container.LogConfig{Config: wantLogs}, got.LogConfig)
	require.Equal(t, cfg.Memory, got.Memory)
	if wantTmpfs == "" {
		require.NotContains(t, got.Tmpfs, "/tmp")
	} else {
		require.Equal(t, wantTmpfs, got.Tmpfs["/tmp"])
	}
	if len(cfg.Mounts) > 0 {
		require.Len(t, got.Mounts, 1)
		require.Equal(t, cfg.Mounts[0].Source, got.Mounts[0].Source)
		require.Equal(t, cfg.Mounts[0].Target, got.Mounts[0].Target)
		require.Equal(t, cfg.Mounts[0].ReadOnly, got.Mounts[0].ReadOnly)
	}
}
