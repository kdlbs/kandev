package api

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-QUESTION-IMAGES-002.2
func TestQuestionImageEndpoint(t *testing.T) {
	srv, mgr, root := newWorkspacePreviewServer(t)
	var pngBytes, jpgBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 8, 8))))
	require.NoError(t, jpeg.Encode(&jpgBytes, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil))
	require.NoError(t, os.WriteFile(filepath.Join(root, "good.PNG"), pngBytes.Bytes(), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "good.jpg"), jpgBytes.Bytes(), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "wrong.jpg"), pngBytes.Bytes(), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "broken.png"), pngBytes.Bytes()[:35], 0600))
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.png"), pngBytes.Bytes(), 0600))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(root, "escape.png")))
	require.NoError(t, os.Symlink("good.PNG", filepath.Join(root, "safe.png")))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "attached")))
	get := func(path, repo string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/workspace/image-preview?"+url.Values{"path": {path}, "repo": {repo}}.Encode(), nil)
		rec := httptest.NewRecorder()
		srv.Router().ServeHTTP(rec, req)
		return rec
	}
	for _, name := range []string{"good.PNG", "good.jpg", "safe.png"} {
		rec := get(name, "")
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Contains(t, rec.Header().Get("Content-Type"), "image/")
		require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
		require.Equal(t, "private, no-store", rec.Header().Get("Cache-Control"))
	}
	for _, name := range []string{"wrong.jpg", "broken.png", "escape.png", "../secret.png", filepath.Join(outside, "secret.png"), "https://example.com/a.png", "~/a.png", "missing.png"} {
		rec := get(name, "")
		require.NotEqual(t, http.StatusOK, rec.Code, name)
		require.NotContains(t, rec.Body.String(), outside)
	}
	require.NotEqual(t, http.StatusOK, get("secret.png", "attached").Code)
	mgr.SetWorkspaceSourceRoots([]string{outside})
	require.Equal(t, http.StatusOK, get("secret.png", "attached").Code)
	mgr.SetWorkspaceSourceRoots(nil)
	require.NotEqual(t, http.StatusOK, get("secret.png", "attached").Code)
}
