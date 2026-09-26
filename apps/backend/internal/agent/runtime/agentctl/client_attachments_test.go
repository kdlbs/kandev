package client

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// parseMaterializeUpload re-reads the multipart body the client streamed so
// tests can assert the exact fields and file bytes agentctl will see.
func parseMaterializeUpload(t *testing.T, got *capturedRequest) (map[string]string, string, []byte) {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(got.ContentType)
	if err != nil {
		t.Fatalf("parse content-type %q: %v", got.ContentType, err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("media type = %q, want multipart/form-data", mediaType)
	}
	reader := multipart.NewReader(strings.NewReader(string(got.Body)), params["boundary"])

	fields := map[string]string{}
	var fileName string
	var fileBytes []byte
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read multipart part: %v", err)
		}
		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part %q: %v", part.FormName(), err)
		}
		if part.FormName() == "file" {
			fileName, fileBytes = part.FileName(), body
			continue
		}
		fields[part.FormName()] = string(body)
	}
	return fields, fileName, fileBytes
}

func TestMaterializeAttachment_StreamsMultipartWithEveryDescriptorField(t *testing.T) {
	srv, got := captureServer(t, jsonResponder(http.StatusOK,
		`{"name":"screenshot-1.png","size_bytes":11}`))

	content := "PNGCONTENT"
	result, err := newHTTPOnlyClient(srv.URL).MaterializeAttachment(
		context.Background(),
		"sess-1", "att-1", "screenshot.png", "image/png",
		int64(len(content)+1), strings.NewReader(content+"!"),
	)
	if err != nil {
		t.Fatalf("MaterializeAttachment: %v", err)
	}

	if got.Method != http.MethodPost || got.Path != "/api/v1/attachments/materialize" {
		t.Errorf("request = %s %s, want POST /api/v1/attachments/materialize", got.Method, got.Path)
	}

	fields, fileName, fileBytes := parseMaterializeUpload(t, got)
	if fields["session_id"] != "sess-1" || fields["attachment_id"] != "att-1" {
		t.Errorf("session/attachment = %q / %q", fields["session_id"], fields["attachment_id"])
	}
	if fields["name"] != "screenshot.png" || fields["mime_type"] != "image/png" {
		t.Errorf("name/mime = %q / %q", fields["name"], fields["mime_type"])
	}
	if fields["size_bytes"] != "11" {
		t.Errorf("size_bytes = %q, want 11", fields["size_bytes"])
	}
	if fileName != "screenshot.png" {
		t.Errorf("file part filename = %q, want screenshot.png", fileName)
	}
	if string(fileBytes) != content+"!" {
		t.Errorf("file bytes = %q, want the raw content streamed verbatim", fileBytes)
	}

	// The client renames server-side; the returned name is authoritative.
	if result.Name != "screenshot-1.png" {
		t.Errorf("Name = %q, want the server-chosen screenshot-1.png", result.Name)
	}
	if result.SizeBytes != 11 {
		t.Errorf("SizeBytes = %d, want 11", result.SizeBytes)
	}
}

func TestMaterializeAttachment_RejectsInvalidArgumentsBeforeAnyRequest(t *testing.T) {
	tests := []struct {
		name         string
		sessionID    string
		attachmentID string
		sizeBytes    int64
		content      io.Reader
		wantErr      string
	}{
		{
			name:         "blank session",
			sessionID:    "   ",
			attachmentID: "att-1",
			content:      strings.NewReader("x"),
			wantErr:      "attachment session, id, and content are required",
		},
		{
			name:         "blank attachment id",
			sessionID:    "sess-1",
			attachmentID: "",
			content:      strings.NewReader("x"),
			wantErr:      "attachment session, id, and content are required",
		},
		{
			name:         "nil content",
			sessionID:    "sess-1",
			attachmentID: "att-1",
			content:      nil,
			wantErr:      "attachment session, id, and content are required",
		},
		{
			name:         "negative size",
			sessionID:    "sess-1",
			attachmentID: "att-1",
			sizeBytes:    -1,
			content:      strings.NewReader("x"),
			wantErr:      "attachment size exceeds maximum",
		},
		{
			name:         "over the descriptor limit",
			sessionID:    "sess-1",
			attachmentID: "att-1",
			sizeBytes:    models.MaxMessageAttachmentBytes + 1,
			content:      strings.NewReader("x"),
			wantErr:      "attachment size exceeds maximum",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var requests int
			srv, _ := captureServer(t, func(w http.ResponseWriter, _ *http.Request) {
				requests++
				w.WriteHeader(http.StatusOK)
			})

			result, err := newHTTPOnlyClient(srv.URL).MaterializeAttachment(
				context.Background(), tc.sessionID, tc.attachmentID, "n", "text/plain",
				tc.sizeBytes, tc.content)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if result != nil {
				t.Errorf("result = %+v, want nil", result)
			}
			if requests != 0 {
				t.Errorf("sent %d requests, want the validation to short-circuit", requests)
			}
		})
	}
}

// The exact limit must be accepted — an off-by-one here silently rejects the
// largest legal attachment.
func TestMaterializeAttachment_AcceptsExactlyTheDescriptorLimit(t *testing.T) {
	srv, _ := captureServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Drain without buffering the whole (large) declared size.
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w,
			`{"name":"big.bin","size_bytes":`+
				strconv.FormatInt(models.MaxMessageAttachmentBytes, 10)+`}`)
	})

	result, err := newHTTPOnlyClient(srv.URL).MaterializeAttachment(
		context.Background(), "sess-1", "att-1", "big.bin", "application/octet-stream",
		models.MaxMessageAttachmentBytes, strings.NewReader("stub"))
	if err != nil {
		t.Fatalf("MaterializeAttachment at the limit: %v", err)
	}
	if result.SizeBytes != models.MaxMessageAttachmentBytes {
		t.Errorf("SizeBytes = %d, want %d", result.SizeBytes, models.MaxMessageAttachmentBytes)
	}
}

func TestMaterializeAttachment_FailsOnNonSuccessStatusAndEchoesBody(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusRequestEntityTooLarge,
		"  attachment exceeds session quota  "))

	_, err := newHTTPOnlyClient(srv.URL).MaterializeAttachment(
		context.Background(), "sess-1", "att-1", "n", "text/plain", 1, strings.NewReader("x"))
	if err == nil {
		t.Fatal("MaterializeAttachment returned nil error for 413")
	}
	if !strings.Contains(err.Error(), "materialize attachment failed with status 413") {
		t.Errorf("error = %v, want status 413 named", err)
	}
	if !strings.Contains(err.Error(), "attachment exceeds session quota") {
		t.Errorf("error = %v, want the server body echoed", err)
	}
	if strings.Contains(err.Error(), "  attachment") {
		t.Errorf("error = %v, want the echoed body trimmed", err)
	}
}

func TestMaterializeAttachment_RejectsMalformedResponseBody(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"name":`))

	_, err := newHTTPOnlyClient(srv.URL).MaterializeAttachment(
		context.Background(), "sess-1", "att-1", "n", "text/plain", 1, strings.NewReader("x"))
	if err == nil || !strings.Contains(err.Error(), "decode materialized attachment") {
		t.Fatalf("error = %v, want decode failure", err)
	}
}

// A response that disagrees with the descriptor means the file on the agentctl
// side is not the one the backend claimed — the caller must not treat it as
// materialized.
func TestMaterializeAttachment_RejectsInconsistentResponseMetadata(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"blank name", `{"name":"   ","size_bytes":5}`},
		{"missing name", `{"size_bytes":5}`},
		{"size mismatch", `{"name":"a.txt","size_bytes":4}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := captureServer(t, jsonResponder(http.StatusOK, tc.body))

			result, err := newHTTPOnlyClient(srv.URL).MaterializeAttachment(
				context.Background(), "sess-1", "att-1", "a.txt", "text/plain",
				5, strings.NewReader("12345"))
			if err == nil || !strings.Contains(err.Error(), "materialize attachment returned invalid metadata") {
				t.Fatalf("error = %v, want invalid metadata", err)
			}
			if result != nil {
				t.Errorf("result = %+v, want nil", result)
			}
		})
	}
}

func TestMaterializeAttachment_HonoursContextCancellation(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusOK, `{"name":"a","size_bytes":1}`))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newHTTPOnlyClient(srv.URL).MaterializeAttachment(
		ctx, "sess-1", "att-1", "a.txt", "text/plain", 1, strings.NewReader("x"))
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("error = %v, want context canceled", err)
	}
}

// A reader that fails mid-copy must propagate through the pipe rather than
// hanging the request or silently truncating the upload.
type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestMaterializeAttachment_PropagatesContentReadFailure(t *testing.T) {
	// The pipe writer is closed with the reader's error, so the request body
	// aborts mid-stream. Use a bare handler: the capture helper would report
	// the (expected) truncated read as a test failure of its own.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	_, err := newHTTPOnlyClient(srv.URL).MaterializeAttachment(
		context.Background(), "sess-1", "att-1", "a.txt", "text/plain",
		1, failingReader{err: io.ErrUnexpectedEOF})
	if err == nil {
		t.Fatal("MaterializeAttachment returned nil error for a failing content reader")
	}
	if !strings.Contains(err.Error(), "materialize attachment") {
		t.Errorf("error = %v, want the upload failure wrapped", err)
	}
}
