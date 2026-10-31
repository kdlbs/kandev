package webapp

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func compressedAssetFixture(t *testing.T) (fstest.MapFS, []byte, []byte) {
	t.Helper()
	original := bytes.Repeat([]byte("export const value = 1;\n"), 100)
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	_, err := writer.Write(original)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	compressed := output.Bytes()
	modified := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return fstest.MapFS{
		"assets/app.js":    {Data: original, ModTime: modified},
		"assets/app.js.gz": {Data: compressed, ModTime: modified},
		"index.html":       {Data: []byte("<html><head></head><body></body></html>")},
	}, original, compressed
}

// @covers AC-PLATFORM-WEB-ASSET-DELIVERY-001.1
func TestAssetCompressionNegotiation(t *testing.T) {
	cases := []struct {
		header string
		gzip   bool
		status int
	}{
		{"", false, 200}, {"gzip", true, 200}, {"br, gzip", true, 200},
		{"GZip;Q=1", true, 200}, {"notgzip", false, 200}, {"gzip;q=0", false, 200},
		{"gzip;q=0.5, identity;q=0.8", false, 200}, {"gzip;q=0.8, identity;q=0.5", true, 200},
		{"gzip;q=0.5, identity;q=0.5", true, 200}, {"*;q=1", true, 200},
		{"gzip;q=0.8, *;q=0.1", false, 200},
		{"gzip;q=0, *;q=1", false, 200}, {"*;q=0", false, 406},
		{"identity;q=0, gzip;q=0", false, 406}, {"identity;q=0, gzip;q=1", true, 200},
		{"gzip;q=bogus", false, 200}, {"gzip;q=2, identity;q=0", false, 406},
	}
	for _, item := range cases {
		t.Run(item.header, func(t *testing.T) {
			assets, original, compressed := compressedAssetFixture(t)
			response := httptest.NewRecorder()
			response.Header().Set("Vary", "Origin")
			request := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
			request.Header.Set("Accept-Encoding", item.header)
			NewHandler(assets).ServeHTTP(response, request)
			require.Equal(t, item.status, response.Code)
			require.Contains(t, strings.Join(response.Header().Values("Vary"), ","), "Accept-Encoding")
			require.Contains(t, strings.Join(response.Header().Values("Vary"), ","), "Origin")
			if item.status != http.StatusOK {
				require.Empty(t, response.Header().Get("Content-Encoding"))
				return
			}
			expected := original
			if item.gzip {
				expected = compressed
				require.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
			} else {
				require.Empty(t, response.Header().Get("Content-Encoding"))
			}
			require.Equal(t, expected, response.Body.Bytes())
			require.Contains(t, response.Header().Get("Content-Type"), "javascript")
			require.Contains(t, response.Header().Get("Cache-Control"), "immutable")
			require.Equal(t, strconv.Itoa(len(expected)), response.Header().Get("Content-Length"))
		})
	}
}

type unseekableAssets struct{ fs.FS }
type unseekableAsset struct{ fs.File }

func (f unseekableAssets) Open(name string) (fs.File, error) {
	file, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	return unseekableAsset{file}, nil
}

// @covers AC-PLATFORM-WEB-ASSET-DELIVERY-001.3
func TestCompressedAssetHTTPBehavior(t *testing.T) {
	for _, nonseekable := range []bool{false, true} {
		t.Run(strconv.FormatBool(nonseekable), func(t *testing.T) {
			assets, _, compressed := compressedAssetFixture(t)
			var source fs.FS = assets
			if nonseekable {
				source = unseekableAssets{source}
			}
			handler := NewHandler(source)
			for _, item := range []struct {
				method, header, value string
				status                int
				body                  []byte
			}{
				{http.MethodHead, "", "", 200, nil},
				{http.MethodGet, "Range", "bytes=0-9", 206, compressed[:10]},
				{http.MethodGet, "Range", "bytes=0-9,20-29", 200, compressed},
				{http.MethodGet, "Range", "bytes=99999-", 416, nil},
				{http.MethodGet, "If-Modified-Since", "Fri, 02 Jan 2026 03:04:05 GMT", 304, nil},
			} {
				t.Run(item.method+item.header, func(t *testing.T) {
					request := httptest.NewRequest(item.method, "/assets/app.js", nil)
					request.Header.Set("Accept-Encoding", "gzip")
					if item.header != "" {
						request.Header.Set(item.header, item.value)
					}
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, request)
					require.Equal(t, item.value, request.Header.Get(item.header))
					require.Equal(t, item.status, response.Code)
					if item.status == 416 {
						require.Empty(t, response.Header().Get("Content-Encoding"))
						require.Contains(t, response.Body.String(), "range")
						return
					}
					require.Equal(t, item.body, response.Body.Bytes())
					if item.method == http.MethodHead {
						require.Equal(t, strconv.Itoa(len(compressed)), response.Header().Get("Content-Length"))
					}
					require.Contains(t, strings.Join(response.Header().Values("Vary"), ","), "Accept-Encoding")
				})
			}
		})
	}
}

// @covers AC-PLATFORM-WEB-ASSET-DELIVERY-001.2
func TestAssetCompressionFallbackAndShell(t *testing.T) {
	assets, original, _ := compressedAssetFixture(t)
	delete(assets, "assets/app.js.gz")
	assets["assets/font.woff2"] = &fstest.MapFile{Data: original}
	handler := NewHandler(assets)
	for _, item := range []struct {
		target, encoding string
		status           int
	}{
		{"/assets/app.js", "gzip", 200}, {"/assets/app.js", "identity;q=0, gzip", 406},
		{"/assets/font.woff2", "gzip", 200}, {"/assets/font.woff2", "identity;q=0", 406},
		{"/assets/font.woff2", "*;q=0", 406},
		{"/assets/missing.js", "gzip", 404}, {"/threads", "gzip", 200},
	} {
		t.Run(item.target+item.encoding, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, item.target, nil)
			request.Header.Set("Accept-Encoding", item.encoding)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, item.status, response.Code)
			require.Empty(t, response.Header().Get("Content-Encoding"))
			if item.target == "/threads" {
				require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			} else if item.status == 200 {
				require.Equal(t, original, response.Body.Bytes())
			}
		})
	}
	assets["assets/orphan.js.gz"] = &fstest.MapFile{Data: []byte("orphan")}
	request := httptest.NewRequest(http.MethodGet, "/assets/orphan.js", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, 404, response.Code)
}

var _ io.Reader = unseekableAsset{}

// @covers AC-PLATFORM-WEB-ASSET-DELIVERY-001.3
func TestAssetFailedPrecondition(t *testing.T) {
	for _, encoding := range []string{"gzip", "identity"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			for _, condition := range []struct{ header, value string }{
				{"If-Match", `"missing-etag"`},
				{"If-Unmodified-Since", "Thu, 01 Jan 2026 03:04:05 GMT"},
			} {
				t.Run(encoding+method+condition.header, func(t *testing.T) {
					assets, _, _ := compressedAssetFixture(t)
					request := httptest.NewRequest(method, "/assets/app.js", nil)
					request.Header.Set("Accept-Encoding", encoding)
					request.Header.Set(condition.header, condition.value)
					response := httptest.NewRecorder()
					NewHandler(assets).ServeHTTP(response, request)
					require.Equal(t, http.StatusPreconditionFailed, response.Code)
					require.Empty(t, response.Body.Bytes())
					require.Empty(t, response.Header().Get("Content-Length"))
					require.Empty(t, response.Header().Get("Content-Encoding"))
				})
			}
		}
	}
}
