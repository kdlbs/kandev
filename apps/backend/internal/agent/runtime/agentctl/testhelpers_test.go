package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// Shared HTTP test scaffolding for the whole package. Every endpoint in this
// client is request-construction plus response-decoding, so the tests need to
// assert what actually went on the wire — not merely that no error came back.

// capturedRequest records everything the client put on the wire so each test
// can assert the request rather than just the decoded response.
type capturedRequest struct {
	Method      string
	Path        string
	RawQuery    string
	Query       url.Values
	ContentType string
	Body        []byte
}

// captureServer serves `respond` and records the single request it receives.
func captureServer(t *testing.T, respond http.HandlerFunc) (*httptest.Server, *capturedRequest) {
	t.Helper()
	got := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		got.Method = r.Method
		got.Path = r.URL.Path
		got.RawQuery = r.URL.RawQuery
		got.Query = r.URL.Query()
		got.ContentType = r.Header.Get("Content-Type")
		got.Body = body
		respond(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

// jsonResponder writes a fixed status and raw JSON body.
func jsonResponder(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}
