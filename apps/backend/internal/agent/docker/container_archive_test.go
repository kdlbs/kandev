package docker

import (
	"archive/tar"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newArchiveTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	daemon := httptest.NewServer(handler)
	t.Cleanup(daemon.Close)

	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	client, err := NewClient(config.DockerConfig{
		Host:       "tcp://" + strings.TrimPrefix(daemon.URL, "http://"),
		APIVersion: "1.44",
	}, log)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// TestCopyToContainerExtractsAtTheRequestedPath pins the request the daemon
// receives: the container's archive endpoint, the destination as a query
// parameter, and the tar stream as the body.
func TestCopyToContainerExtractsAtTheRequestedPath(t *testing.T) {
	var gotMethod, gotPath, gotDest string
	var gotBody []byte

	client := newArchiveTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/_ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotDest = r.URL.Query().Get("path")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})

	archive := tarWithSingleFile(t, "usr/local/bin/agentctl", []byte("helper"), 0o755)
	require.NoError(t, client.CopyToContainer(context.Background(), "abc123", "/", archive))

	require.Equal(t, http.MethodPut, gotMethod)
	require.True(t, strings.HasSuffix(gotPath, "/containers/abc123/archive"), "path was %q", gotPath)
	require.Equal(t, "/", gotDest)
	require.Equal(t, archive, gotBody, "the daemon must receive the tar stream unmodified")
}

// TestCopyToContainerReportsDaemonFailure keeps a rejected extraction a
// failure. Swallowing it would start a container whose helper never arrived.
func TestCopyToContainerReportsDaemonFailure(t *testing.T) {
	client := newArchiveTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/_ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"No such container: abc123"}`))
	})

	err := client.CopyToContainer(context.Background(), "abc123", "/", []byte("not-a-tar"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "abc123")
}

func tarWithSingleFile(t *testing.T, name string, data []byte, mode int64) []byte {
	t.Helper()
	var buf strings.Builder
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Mode:     mode,
		Size:     int64(len(data)),
	}))
	_, err := tw.Write(data)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	return []byte(buf.String())
}
