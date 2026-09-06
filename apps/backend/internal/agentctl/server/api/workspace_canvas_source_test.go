package api

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func newCanvasSourceTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	workDir := t.TempDir()
	log := newTestLogger()
	cfg := &config.InstanceConfig{
		Port:       0,
		WorkDir:    workDir,
		AuthToken:  "canvas-secret",
		InstanceID: "instance-1",
	}
	return NewServer(cfg, process.NewManager(cfg, log), nil, nil, log), workDir
}

func canvasSourceRequest(t *testing.T, server *Server, root string, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(agentctltypes.CanvasSourceTransferRequest{Root: root})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, agentctltypes.CanvasSourceTransferPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(InstanceIDHeader, "instance-1")
	if auth {
		req.Header.Set("Authorization", "Bearer canvas-secret")
	}
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, req)
	return response
}

func readCanvasTar(t *testing.T, data []byte) map[string]string {
	t.Helper()
	reader := tar.NewReader(bytes.NewReader(data))
	entries := make(map[string]string)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return entries
		}
		require.NoError(t, err)
		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		entries[header.Name] = string(content)
	}
}

func TestCanvasSourceTransfer_RequiresBearerAuthentication(t *testing.T) {
	server, workDir := newCanvasSourceTestServer(t)
	require.NoError(t, os.Mkdir(filepath.Join(workDir, "source"), 0o755))

	response := canvasSourceRequest(t, server, "source", false)
	require.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestCanvasSourceTransfer_StreamsWorkspaceRelativeRoot(t *testing.T) {
	server, workDir := newCanvasSourceTestServer(t)
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, "source", "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "source", ".canvas-root"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "source", "index.html"), []byte("<main>ok</main>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "source", "assets", "app.js"), []byte("console.log(1)"), 0o644))

	response := canvasSourceRequest(t, server, "source", true)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, agentctltypes.CanvasSourceContentType, response.Header().Get("Content-Type"))
	require.Equal(t, map[string]string{
		"index.html":    "<main>ok</main>",
		"assets/":       "",
		"assets/app.js": "console.log(1)",
	}, readCanvasTar(t, response.Body.Bytes()))
}

func TestCanvasSourceTransfer_RejectsTraversalAndSymlinks(t *testing.T) {
	server, workDir := newCanvasSourceTestServer(t)
	require.NoError(t, os.Mkdir(filepath.Join(workDir, "source"), 0o755))
	outside := filepath.Join(workDir, "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))

	for _, root := range []string{"../", filepath.Join(string(os.PathSeparator), "tmp", "outside")} {
		response := canvasSourceRequest(t, server, root, true)
		require.Equal(t, http.StatusBadRequest, response.Code, "root %q", root)
	}

	require.NoError(t, os.Symlink(outside, filepath.Join(workDir, "source", "linked.txt")))
	response := canvasSourceRequest(t, server, "source", true)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.NotContains(t, response.Body.String(), "secret")
}

func TestCanvasSourceTransfer_RejectsFileCountLimit(t *testing.T) {
	server, workDir := newCanvasSourceTestServer(t)
	root := filepath.Join(workDir, "source")
	require.NoError(t, os.Mkdir(root, 0o755))
	for i := 0; i < agentctltypes.MaxCanvasSourceFiles+1; i++ {
		name := filepath.Join(root, fmt.Sprintf("file-%04d.txt", i))
		require.NoError(t, os.WriteFile(name, nil, 0o644))
	}

	response := canvasSourceRequest(t, server, "source", true)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
}

func TestCanvasSourceTransfer_RejectsFileDataLimit(t *testing.T) {
	server, workDir := newCanvasSourceTestServer(t)
	root := filepath.Join(workDir, "source")
	require.NoError(t, os.Mkdir(root, 0o755))
	file, err := os.Create(filepath.Join(root, "large.bin"))
	require.NoError(t, err)
	require.NoError(t, file.Truncate(int64(agentctltypes.MaxCanvasSourceFileData+1)))
	require.NoError(t, file.Close())

	response := canvasSourceRequest(t, server, "source", true)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
}
