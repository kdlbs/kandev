package websocket

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

const proxyPrefix = "/port-proxy/abc/3001"

func TestRewriteAbsolutePath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"path-absolute", "/foo/bar.js", proxyPrefix + "/foo/bar.js"},
		{"root", "/", proxyPrefix + "/"},
		{"network-relative skipped", "//cdn.example.com/x", "//cdn.example.com/x"},
		{"absolute http skipped", "http://example.com/x", "http://example.com/x"},
		{"relative skipped", "foo.js", "foo.js"},
		{"dot-relative skipped", "./foo.js", "./foo.js"},
		{"parent-relative skipped", "../foo.js", "../foo.js"},
		{"data-uri skipped", "data:image/png;base64,xyz", "data:image/png;base64,xyz"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rewriteAbsolutePath(c.in, proxyPrefix)
			if got != c.want {
				t.Fatalf("rewriteAbsolutePath(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestRewriteHTMLURLs(t *testing.T) {
	in := `<!DOCTYPE html>
<html>
<head>
<link rel="stylesheet" href="/styles/main.css">
<link rel="stylesheet" href="//cdn.example.com/lib.css">
<script src="/static/app.js"></script>
<style>body { background: url(/img/bg.png); }</style>
</head>
<body>
<img src="/img/logo.png" srcset="/img/logo@2x.png 2x, https://cdn.example.com/logo.png 3x">
<a href="/about">About</a>
<a href="http://external.example.com/x">External</a>
<form action="/submit"><input formaction="/quick"></form>
<div style="background: url('/bg.jpg');"></div>
</body>
</html>`

	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix))

	mustContain(t, got, `href="/port-proxy/abc/3001/styles/main.css"`)
	mustContain(t, got, `href="//cdn.example.com/lib.css"`)
	mustContain(t, got, `src="/port-proxy/abc/3001/static/app.js"`)
	mustContain(t, got, `src="/port-proxy/abc/3001/img/logo.png"`)
	mustContain(t, got, `/port-proxy/abc/3001/img/logo@2x.png 2x`)
	mustContain(t, got, `https://cdn.example.com/logo.png 3x`)
	mustContain(t, got, `href="/port-proxy/abc/3001/about"`)
	mustContain(t, got, `href="http://external.example.com/x"`)
	mustContain(t, got, `action="/port-proxy/abc/3001/submit"`)
	mustContain(t, got, `formaction="/port-proxy/abc/3001/quick"`)
	// Inline style="url('/bg.jpg')" — html package HTML-escapes single quotes
	// on serialization, so check for the rewritten path without the quote.
	mustContain(t, got, `/port-proxy/abc/3001/bg.jpg`)
	// Inline <style> block should be rewritten via rewriteCSSFragment.
	mustContain(t, got, `url(/port-proxy/abc/3001/img/bg.png)`)
}

func TestRewriteCSSURLs(t *testing.T) {
	in := `@import "/theme.css";
@import url("/print.css");
.bg { background: url(/img/bg.png) no-repeat; }
.cdn { background: url("//cdn.example.com/x.png"); }
.abs { background: url(http://example.com/x.png); }
.rel { background: url(foo.png); }`

	got := string(rewriteCSSURLs([]byte(in), proxyPrefix))

	mustContain(t, got, `@import "/port-proxy/abc/3001/theme.css"`)
	mustContain(t, got, `url("/port-proxy/abc/3001/print.css")`)
	mustContain(t, got, `url(/port-proxy/abc/3001/img/bg.png)`)
	mustContain(t, got, `url("//cdn.example.com/x.png")`)
	mustContain(t, got, `url(http://example.com/x.png)`)
	mustContain(t, got, `url(foo.png)`)
}

func TestRewriteProxyResponse_HTML(t *testing.T) {
	body := `<a href="/x">x</a>`
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:   io.NopCloser(strings.NewReader(body)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), `href="/port-proxy/abc/3001/x"`) {
		t.Fatalf("HTML not rewritten: %q", got)
	}
	if resp.ContentLength != int64(len(got)) {
		t.Fatalf("ContentLength mismatch: %d vs %d", resp.ContentLength, len(got))
	}
}

func TestRewriteProxyResponse_CSS(t *testing.T) {
	body := `body { background: url(/bg.png); }`
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"text/css"}},
		Body:   io.NopCloser(strings.NewReader(body)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), `url(/port-proxy/abc/3001/bg.png)`) {
		t.Fatalf("CSS not rewritten: %q", got)
	}
}

func TestRewriteProxyResponse_OtherContentTypeUnchanged(t *testing.T) {
	body := `{"href":"/foo"}`
	resp := &http.Response{
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(body)),
	}
	if err := rewriteProxyResponse(resp, proxyPrefix); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != body {
		t.Fatalf("non-HTML/CSS response was modified: %q", got)
	}
}

func TestRewriteHTMLURLs_InjectsRuntimeShim(t *testing.T) {
	in := `<!DOCTYPE html><html><head><title>x</title></head><body></body></html>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix))

	// The shim must appear exactly once, immediately after the `<head>` open tag
	// (so it executes before any other script that may follow).
	const marker = "window.fetch="
	if strings.Count(got, marker) != 1 {
		t.Fatalf("expected exactly one runtime shim, got %d copies\n%s",
			strings.Count(got, marker), got)
	}
	headIdx := strings.Index(got, "<head>")
	titleIdx := strings.Index(got, "<title>")
	shimIdx := strings.Index(got, marker)
	if headIdx >= shimIdx || shimIdx >= titleIdx {
		t.Fatalf("shim must come between <head> and <title>: head=%d shim=%d title=%d\n%s",
			headIdx, shimIdx, titleIdx, got)
	}
	// The prefix must be baked into the JS literal.
	if !strings.Contains(got, `var P="/port-proxy/abc/3001"`) {
		t.Fatalf("shim missing baked-in prefix:\n%s", got)
	}
}

func TestRewriteHTMLURLs_NoHeadStillRewritesURLs(t *testing.T) {
	// Documents without a <head> are rare but possible. We don't bother
	// injecting the shim in that case (no good anchor point), but URL
	// rewriting must still work.
	in := `<a href="/foo">x</a>`
	got := string(rewriteHTMLURLs([]byte(in), proxyPrefix))
	if !strings.Contains(got, `href="/port-proxy/abc/3001/foo"`) {
		t.Fatalf("URL not rewritten: %q", got)
	}
	if strings.Contains(got, "window.fetch=") {
		t.Fatalf("unexpected shim in headless document: %q", got)
	}
}

func TestRewriteSrcSet(t *testing.T) {
	in := "/a.png 1x, /b.png 2x, //cdn.example.com/c.png 3x"
	got := rewriteSrcSet(in, proxyPrefix)
	want := "/port-proxy/abc/3001/a.png 1x, /port-proxy/abc/3001/b.png 2x, //cdn.example.com/c.png 3x"
	if got != want {
		t.Fatalf("rewriteSrcSet = %q, want %q", got, want)
	}
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("output missing %q\noutput: %s", needle, haystack)
	}
}
