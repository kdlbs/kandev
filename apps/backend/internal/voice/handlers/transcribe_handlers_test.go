package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/voice/transcribe"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "text", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("logger.NewLogger: %v", err)
	}
	return log
}

func buildAudioRequest(t *testing.T, field, filename, mime string, data []byte) (*http.Request, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	if data != nil {
		fw, err := createFormFile(w, field, filename, mime)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(data)
	}
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transcribe", buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req, w.FormDataContentType()
}

func createFormFile(w *multipart.Writer, field, filename, mime string) (interface{ Write([]byte) (int, error) }, error) {
	if mime == "" {
		return w.CreateFormFile(field, filename)
	}
	hdr := make(map[string][]string)
	hdr["Content-Disposition"] = []string{"form-data; name=\"" + field + "\"; filename=\"" + filename + "\""}
	hdr["Content-Type"] = []string{mime}
	return w.CreatePart(hdr)
}

func newRouter(svc *transcribe.Service, t *testing.T) *gin.Engine {
	r := gin.New()
	RegisterRoutes(r, svc, testLogger(t))
	return r
}

func TestTranscribe_NotConfigured(t *testing.T) {
	svc := transcribe.New("")
	r := newRouter(svc, t)

	req, _ := buildAudioRequest(t, "audio", "a.webm", "audio/webm", []byte("hello"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", w.Code, w.Body.String())
	}
}

func TestTranscribe_MissingFile(t *testing.T) {
	svc := transcribe.New("sk-test")
	r := newRouter(svc, t)

	// No file part — just an empty form.
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transcribe", buf)
	req.Header.Set("Content-Type", w.FormDataContentType())

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestTranscribe_EmptyAudio(t *testing.T) {
	svc := transcribe.New("sk-test")
	r := newRouter(svc, t)

	req, _ := buildAudioRequest(t, "audio", "a.webm", "audio/webm", []byte{})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestTranscribe_Success(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("auth header missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"transcribed"}`))
	}))
	defer upstream.Close()

	svc := transcribe.New("sk-test", transcribe.WithEndpoint(upstream.URL))
	r := newRouter(svc, t)

	req, _ := buildAudioRequest(t, "audio", "clip.webm", "audio/webm", []byte("bytes"))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Text != "transcribed" {
		t.Errorf("text = %q", body.Text)
	}
}

func TestTranscribe_UpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"oops"}`))
	}))
	defer upstream.Close()

	svc := transcribe.New("sk-test", transcribe.WithEndpoint(upstream.URL))
	r := newRouter(svc, t)
	req, _ := buildAudioRequest(t, "audio", "a.webm", "audio/webm", []byte("bytes"))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "upstream") {
		t.Errorf("body should mention upstream: %s", rr.Body.String())
	}
}
