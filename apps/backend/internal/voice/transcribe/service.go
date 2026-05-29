// Package transcribe wraps the OpenAI Whisper transcription endpoint for the
// chat voice-input fallback. The browser's Web Speech API is the primary
// voice-input engine; this server-side path is only hit when the browser
// has no SpeechRecognition support.
package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

// ErrNotConfigured is returned when the service has no API key — the handler
// maps this to HTTP 503 so the frontend can hide the Whisper fallback path
// instead of repeatedly retrying a deployment that will never succeed.
var ErrNotConfigured = errors.New("voice transcription is not configured")

// UpstreamError wraps a non-2xx response from OpenAI so the handler can map
// it to HTTP 502 and surface a clean error to the caller.
type UpstreamError struct {
	StatusCode int
	Body       string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("openai whisper upstream error: status=%d body=%s", e.StatusCode, e.Body)
}

const (
	defaultEndpoint = "https://api.openai.com/v1/audio/transcriptions"
	defaultModel    = "whisper-1"
	defaultTimeout  = 60 * time.Second
)

// Service transcribes audio via OpenAI's Whisper endpoint.
type Service struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
}

// Option customises a Service for tests (custom endpoint, HTTP client).
type Option func(*Service)

// WithEndpoint overrides the upstream URL — used by tests with httptest servers.
func WithEndpoint(url string) Option {
	return func(s *Service) { s.endpoint = url }
}

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(s *Service) { s.client = c }
}

// WithModel overrides the Whisper model name.
func WithModel(model string) Option {
	return func(s *Service) { s.model = model }
}

// New constructs a Service. apiKey may be empty; in that case Transcribe
// returns ErrNotConfigured without making any network calls.
func New(apiKey string, opts ...Option) *Service {
	s := &Service{
		apiKey:   apiKey,
		endpoint: defaultEndpoint,
		model:    defaultModel,
		client:   &http.Client{Timeout: defaultTimeout},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Configured reports whether the service has an API key. Used by handlers
// to short-circuit before reading the request body.
func (s *Service) Configured() bool {
	return s != nil && strings.TrimSpace(s.apiKey) != ""
}

// Transcribe sends the given audio bytes to OpenAI Whisper and returns the
// transcribed text. filename is used for the multipart Content-Disposition;
// Whisper relies on the file extension to detect the audio format.
func (s *Service) Transcribe(ctx context.Context, audio []byte, mimeType, filename string) (string, error) {
	if !s.Configured() {
		return "", ErrNotConfigured
	}
	if len(audio) == 0 {
		return "", errors.New("audio payload is empty")
	}

	body, contentType, err := buildMultipart(audio, mimeType, filename, s.model)
	if err != nil {
		return "", fmt.Errorf("build multipart body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, body)
	if err != nil {
		return "", fmt.Errorf("build whisper request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call whisper endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &UpstreamError{StatusCode: resp.StatusCode, Body: string(rawBody)}
	}

	var parsed struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return "", fmt.Errorf("decode whisper response: %w", err)
	}
	return strings.TrimSpace(parsed.Text), nil
}

// buildMultipart assembles the multipart/form-data body Whisper expects:
// `file`, `model`, and `response_format=json`.
func buildMultipart(audio []byte, mimeType, filename, model string) (io.Reader, string, error) {
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)

	if filename == "" {
		filename = "recording" + extensionForMime(mimeType)
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	if mimeType != "" {
		header.Set("Content-Type", mimeType)
	}
	filePart, err := w.CreatePart(header)
	if err != nil {
		return nil, "", err
	}
	if _, err := filePart.Write(audio); err != nil {
		return nil, "", err
	}

	if err := w.WriteField("model", model); err != nil {
		return nil, "", err
	}
	if err := w.WriteField("response_format", "json"); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf, w.FormDataContentType(), nil
}

// extensionForMime maps the audio MIME types MediaRecorder commonly emits to
// the file extensions Whisper recognises. Default to ".webm" — supported by
// Whisper and the most common MediaRecorder default on Chrome.
func extensionForMime(mime string) string {
	mime = strings.ToLower(mime)
	switch {
	case strings.Contains(mime, "wav"):
		return ".wav"
	case strings.Contains(mime, "mp4"), strings.Contains(mime, "m4a"):
		return ".m4a"
	case strings.Contains(mime, "mpeg"), strings.Contains(mime, "mp3"):
		return ".mp3"
	case strings.Contains(mime, "ogg"):
		return ".ogg"
	default:
		return ".webm"
	}
}
