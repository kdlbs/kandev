package webapp

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/webapp/embedded"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-WEB-ASSET-DELIVERY-001.4
func TestEmbeddedCompressedAsset(t *testing.T) {
	assets, err := embedded.FS()
	require.NoError(t, err)
	var name string
	require.NoError(t, fs.WalkDir(assets, ".", func(candidate string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "" && !entry.IsDir() && strings.HasSuffix(candidate, ".js.gz") {
			name = strings.TrimSuffix(candidate, ".gz")
		}
		return nil
	}))
	if name == "" {
		t.Skip("run the Vite build and make sync-embedded-web to verify packaged compression")
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/"+name, nil)
	request.Header.Set("Accept-Encoding", "gzip")
	NewHandler(assets).ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
	reader, err := gzip.NewReader(bytes.NewReader(response.Body.Bytes()))
	require.NoError(t, err)
	defer func() { require.NoError(t, reader.Close()) }()
	decoded, err := io.ReadAll(reader)
	require.NoError(t, err)
	original, err := fs.ReadFile(assets, name)
	require.NoError(t, err)
	require.Equal(t, original, decoded)
	t.Logf("embedded %s: identity=%d gzip=%d bytes", name, len(original), response.Body.Len())
}
