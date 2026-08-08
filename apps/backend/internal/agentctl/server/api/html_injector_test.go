package api

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestInjectInspectorScript_BeforeBodyClose(t *testing.T) {
	html := []byte("<html><body><p>Hello</p></body></html>")
	result := injectInspectorScript(html)
	s := string(result)
	if !strings.Contains(s, "<script>") {
		t.Fatal("expected <script> tag to be injected")
	}
	scriptIdx := strings.Index(s, "<script>")
	bodyIdx := strings.Index(strings.ToLower(s), "</body>")
	if scriptIdx >= bodyIdx {
		t.Fatal("script must appear before </body>")
	}
}

func TestInjectInspectorScript_NoBodyTag(t *testing.T) {
	html := []byte("<p>No body tag</p>")
	result := injectInspectorScript(html)
	if !strings.Contains(string(result), "<script>") {
		t.Fatal("script should be appended even without </body>")
	}
}

func TestInjectInspectorScript_UpperCaseBodyTag(t *testing.T) {
	html := []byte("<html><body><p>Hello</p></BODY></html>")
	result := injectInspectorScript(html)
	s := string(result)
	scriptIdx := strings.Index(s, "<script>")
	bodyIdx := strings.Index(strings.ToLower(s), "</body>")
	if scriptIdx >= bodyIdx {
		t.Fatal("should handle uppercase </BODY>")
	}
}

func TestInspectorScript_UsesPreviewRouteForAnnotationPagePath(t *testing.T) {
	if !strings.Contains(inspectorScript, "function currentPagePath()") {
		t.Fatal("inspector should derive annotation routes through currentPagePath")
	}
	if !strings.Contains(inspectorScript, "window.__kandevProxyPrefix") {
		t.Fatal("inspector should read the proxy prefix exposed by the runtime shim")
	}
	if !strings.Contains(inspectorScript, "pagePath: currentPagePath()") {
		t.Fatal("annotations should store the app route, not location.pathname directly")
	}
}

func TestInspectorScript_PreservesPendingAnnotationAcrossPopupCleanup(t *testing.T) {
	openStart := strings.Index(inspectorScript, "function openCommentPopup(")
	if openStart < 0 {
		t.Fatal("inspector should define openCommentPopup")
	}
	openEnd := strings.Index(inspectorScript[openStart:], "function commitAnnotation(")
	if openEnd < 0 {
		t.Fatal("inspector should define commitAnnotation after openCommentPopup")
	}
	openBody := inspectorScript[openStart : openStart+openEnd]

	keep := strings.Index(openBody, "var keep = pendingAnnotation;")
	cleanup := strings.Index(openBody, "closePopup();")
	restore := strings.Index(openBody, "pendingAnnotation = keep;")
	if keep < 0 || cleanup < 0 || restore < 0 {
		t.Fatal("openCommentPopup should preserve pendingAnnotation across preventive cleanup")
	}
	if keep >= cleanup || cleanup >= restore {
		t.Fatal("openCommentPopup should preserve pendingAnnotation across preventive cleanup")
	}

	closeStart := strings.Index(inspectorScript, "function closePopup()")
	if closeStart < 0 {
		t.Fatal("inspector should define closePopup")
	}
	closeEnd := strings.Index(inspectorScript[closeStart:], "function currentPagePath()")
	if closeEnd < 0 {
		t.Fatal("inspector should define currentPagePath after closePopup")
	}
	if !strings.Contains(inspectorScript[closeStart:closeStart+closeEnd], "pendingAnnotation = null;") {
		t.Fatal("direct popup cleanup should still clear pendingAnnotation")
	}
}

func TestStripIframeSecurityHeaders_RemovesBlockingHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Security-Policy", "default-src 'none'")
	h.Set("Content-Security-Policy-Report-Only", "default-src 'none'")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Type", "text/html")

	stripIframeSecurityHeaders(h)

	if h.Get("Content-Security-Policy") != "" {
		t.Error("CSP should be stripped")
	}
	if h.Get("Content-Security-Policy-Report-Only") != "" {
		t.Error("CSP-Report-Only should be stripped")
	}
	if h.Get("X-Frame-Options") != "" {
		t.Error("X-Frame-Options should be stripped")
	}
	if h.Get("Content-Type") == "" {
		t.Error("Content-Type should not be stripped")
	}
}

func TestInjectScriptsIntoResponse_UpdatesContentLengthAndStripsEncoding(t *testing.T) {
	original := "<html><body><p>Hi</p></body></html>"
	resp := &http.Response{
		Header: http.Header{
			"Content-Type":     []string{"text/html; charset=utf-8"},
			"Content-Encoding": []string{"gzip"},
		},
		Body:          io.NopCloser(strings.NewReader(original)),
		ContentLength: int64(len(original)),
	}

	if err := injectScriptsIntoResponse(resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	if int64(len(body)) != resp.ContentLength {
		t.Errorf("ContentLength mismatch: header=%d actual body=%d", resp.ContentLength, len(body))
	}
	if resp.Header.Get("Content-Encoding") != "" {
		t.Error("Content-Encoding should be deleted after body rewrite")
	}
	if !strings.Contains(string(body), "<script>") {
		t.Error("response body should contain injected <script>")
	}
}
