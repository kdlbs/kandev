package transcribe

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestService_Transcribe_NotConfigured(t *testing.T) {
	svc := New("")
	_, err := svc.Transcribe(context.Background(), []byte("data"), "audio/webm", "")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestService_Configured(t *testing.T) {
	if New("").Configured() {
		t.Errorf("empty key should not be configured")
	}
	if New("   ").Configured() {
		t.Errorf("whitespace-only key should not be configured")
	}
	if !New("sk-test").Configured() {
		t.Errorf("non-empty key should be configured")
	}
}

func TestService_Transcribe_EmptyAudio(t *testing.T) {
	svc := New("sk-test")
	_, err := svc.Transcribe(context.Background(), nil, "audio/webm", "")
	if err == nil {
		t.Fatal("expected error for empty audio")
	}
}

func TestService_Transcribe_Success(t *testing.T) {
	var capturedAuth string
	var capturedFilename string
	var capturedFileBytes []byte
	var capturedModel string
	var capturedFormat string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		capturedModel = r.FormValue("model")
		capturedFormat = r.FormValue("response_format")
		fh := r.MultipartForm.File["file"]
		if len(fh) != 1 {
			t.Fatalf("expected 1 file part, got %d", len(fh))
		}
		capturedFilename = fh[0].Filename
		f, err := fh[0].Open()
		if err != nil {
			t.Fatalf("open file: %v", err)
		}
		defer func() { _ = f.Close() }()
		capturedFileBytes, _ = io.ReadAll(f)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"hello world"}`))
	}))
	defer srv.Close()

	svc := New("sk-test", WithEndpoint(srv.URL))
	text, err := svc.Transcribe(context.Background(), []byte("audio-bytes"), "audio/webm", "clip.webm")
	if err != nil {
		t.Fatalf("Transcribe failed: %v", err)
	}
	if text != "hello world" {
		t.Errorf("unexpected text: %q", text)
	}
	if capturedAuth != "Bearer sk-test" {
		t.Errorf("auth header = %q", capturedAuth)
	}
	if capturedModel != defaultModel {
		t.Errorf("model = %q", capturedModel)
	}
	if capturedFormat != "json" {
		t.Errorf("response_format = %q", capturedFormat)
	}
	if capturedFilename != "clip.webm" {
		t.Errorf("filename = %q", capturedFilename)
	}
	if string(capturedFileBytes) != "audio-bytes" {
		t.Errorf("file body = %q", string(capturedFileBytes))
	}
}

func TestService_Transcribe_DerivedFilename(t *testing.T) {
	var capturedFilename string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(32 << 20)
		fh := r.MultipartForm.File["file"]
		if len(fh) == 1 {
			capturedFilename = fh[0].Filename
		}
		_, _ = w.Write([]byte(`{"text":""}`))
	}))
	defer srv.Close()

	svc := New("sk-test", WithEndpoint(srv.URL))
	_, err := svc.Transcribe(context.Background(), []byte("a"), "audio/wav", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(capturedFilename, ".wav") {
		t.Errorf("derived filename should use .wav for audio/wav, got %q", capturedFilename)
	}
}

func TestService_Transcribe_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad audio"}`))
	}))
	defer srv.Close()

	svc := New("sk-test", WithEndpoint(srv.URL))
	_, err := svc.Transcribe(context.Background(), []byte("a"), "audio/webm", "")
	var upstream *UpstreamError
	if !errors.As(err, &upstream) {
		t.Fatalf("expected UpstreamError, got %T: %v", err, err)
	}
	if upstream.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d", upstream.StatusCode)
	}
	if !strings.Contains(upstream.Body, "bad audio") {
		t.Errorf("body did not contain upstream payload: %q", upstream.Body)
	}
}

func TestExtensionForMime(t *testing.T) {
	cases := map[string]string{
		"audio/webm":           ".webm",
		"audio/wav":            ".wav",
		"audio/x-wav":          ".wav",
		"audio/mp4":            ".m4a",
		"audio/m4a":            ".m4a",
		"audio/mpeg":           ".mp3",
		"audio/mp3":            ".mp3",
		"audio/ogg":            ".ogg",
		"":                     ".webm",
		"application/anything": ".webm",
	}
	for mime, want := range cases {
		if got := extensionForMime(mime); got != want {
			t.Errorf("extensionForMime(%q) = %q, want %q", mime, got, want)
		}
	}
}

func TestBuildMultipart_Roundtrip(t *testing.T) {
	body, ct, err := buildMultipart([]byte("hello"), "audio/wav", "a.wav", "whisper-1")
	if err != nil {
		t.Fatal(err)
	}
	// Parse the multipart body back out using the boundary embedded in ct.
	mediaType, params, ok := splitContentType(ct)
	if !ok || mediaType != "multipart/form-data" {
		t.Fatalf("unexpected content-type: %q", ct)
	}
	mr := multipart.NewReader(body, params["boundary"])
	fields := map[string]string{}
	var fileContent string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		buf, _ := io.ReadAll(part)
		if part.FileName() != "" {
			fileContent = string(buf)
		} else {
			fields[part.FormName()] = string(buf)
		}
	}
	if fileContent != "hello" {
		t.Errorf("file part = %q", fileContent)
	}
	if fields["model"] != "whisper-1" {
		t.Errorf("model field = %q", fields["model"])
	}
	if fields["response_format"] != "json" {
		t.Errorf("response_format field = %q", fields["response_format"])
	}
}

// splitContentType is a tiny helper to split "multipart/form-data; boundary=…"
// without pulling in mime.ParseMediaType — keeps this test file self-contained.
func splitContentType(ct string) (string, map[string]string, bool) {
	parts := strings.SplitN(ct, ";", 2)
	if len(parts) != 2 {
		return "", nil, false
	}
	mediaType := strings.TrimSpace(parts[0])
	params := map[string]string{}
	for _, kv := range strings.Split(parts[1], ";") {
		kv = strings.TrimSpace(kv)
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		params[kv[:eq]] = strings.Trim(kv[eq+1:], `"`)
	}
	return mediaType, params, true
}
