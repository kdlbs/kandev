package client

import (
	"context"
	"github.com/kandev/kandev/internal/common/imagepreview"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestQuestionImageClient(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		mime, body string
		want       error
	}{
		{"ok", 200, "image/png", "png", nil},
		{"old runtime", 404, "text/plain", "404 page not found", imagepreview.Unsupported},
		{"missing file", 404, "application/json", `{"code":"not_found"}`, imagepreview.NotFound},
		{"unsafe MIME", 200, "text/html", "<html>", imagepreview.InvalidImage},
		{"oversize", 200, "image/png", strings.Repeat("x", imagepreview.MaxBytes+1), imagepreview.TooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := captureServer(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.mime)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			result, err := newHTTPOnlyClient(srv.URL).RequestQuestionImage(context.Background(), "a b.png", "repo")
			require.ErrorIs(t, err, tc.want)
			require.Equal(t, "/api/v1/workspace/image-preview", got.Path)
			require.Equal(t, "a b.png", got.Query.Get("path"))
			if tc.want == nil {
				require.Equal(t, []byte(tc.body), result.Bytes)
			} else {
				require.Nil(t, result)
			}
		})
	}
}

func TestQuestionImageClientDoesNotFollowRedirects(t *testing.T) {
	var externalReads atomic.Int32
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		externalReads.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("external"))
	}))
	defer external.Close()
	srv, _ := captureServer(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, external.URL, http.StatusFound) })
	result, err := newHTTPOnlyClient(srv.URL).RequestQuestionImage(t.Context(), "a.png", "")
	require.Error(t, err)
	require.Nil(t, result)
	require.Zero(t, externalReads.Load())
}
