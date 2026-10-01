package controller

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type runtimeSourceTransport func(*http.Request) (*http.Response, error)

func (f runtimeSourceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.1
func TestNativeRuntimeReleaseValidatesVendorMetadata(t *testing.T) {
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	for _, test := range []struct{ body, want string }{
		{`{"tag_name":"v1.52.0","draft":false,"prerelease":false}`, "1.52.0"},
		{`{"tag_name":"v2026.9.24","draft":false,"prerelease":false}`, "2026.9.24"},
		{`{"tag_name":"v2.0.0","draft":true}`, ""},
		{`{"tag_name":"v2.0.0","prerelease":true}`, ""},
		{`{"tag_name":"nightly"}`, ""},
		{`not-json`, ""},
	} {
		http.DefaultClient = &http.Client{Transport: runtimeSourceTransport(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://api.github.com/repos/aaif-goose/goose/releases/latest" {
				t.Errorf("unexpected source: %s", req.URL)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(test.body))}, nil
		})}
		c := newTestController(nil)
		version, err := c.resolveGitHubRuntimeRelease(context.Background(), "aaif-goose/goose")
		if version != test.want || (err == nil) != (test.want != "") {
			t.Errorf("body %s: version %q, err %v", test.body, version, err)
		}
	}
}
