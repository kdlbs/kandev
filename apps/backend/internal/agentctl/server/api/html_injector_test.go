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

func TestInspectorScript_CapturesVersionedTextEvidence(t *testing.T) {
	wants := []string{
		"var PROTOCOL_VERSION = 2;",
		"function captureTextSelection()",
		"selection.getRangeAt(0)",
		"function captureTextEndpoint(",
		"node_path:",
		"range.getClientRects()",
		"containing_element:",
		"send('capture-completed'",
	}
	for _, want := range wants {
		if !strings.Contains(inspectorScript, want) {
			t.Errorf("inspector should include %q", want)
		}
	}
}

func TestInspectorScript_HintsElementCandidateBeforeCapture(t *testing.T) {
	wants := []string{
		"function showCandidate(",
		"candidateLabel.textContent",
		"send('candidate-changed'",
		"document.addEventListener('focusin'",
		"document.addEventListener('touchstart'",
		"case 'element':",
	}
	for _, want := range wants {
		if !strings.Contains(inspectorScript, want) {
			t.Errorf("inspector should include %q", want)
		}
	}
}

func TestInspectorScript_TracksRoutesAndProjectsMarkers(t *testing.T) {
	wants := []string{
		"function currentPageRoute()",
		"window.__kandevProxyPrefix",
		"history.pushState",
		"window.addEventListener('popstate'",
		"case 'project-markers':",
		"function renderMarkers()",
		"send('route-changed'",
	}
	for _, want := range wants {
		if !strings.Contains(inspectorScript, want) {
			t.Errorf("inspector should include %q", want)
		}
	}
}

func TestInspectorScript_SanitizesRoutesAndCompletesTouchKeyboardCaptures(t *testing.T) {
	wants := []string{
		"function sanitizedSearch()",
		"location.search",
		"document.addEventListener('touchend', onTextTouchEnd, true)",
		"event.key === 'Enter'",
		"event.key === ' '",
		"querySelectorAll(selector).length === 1",
		"hideCandidate();",
	}
	for _, want := range wants {
		if !strings.Contains(inspectorScript, want) {
			t.Errorf("inspector should include %q", want)
		}
	}
}

func TestInspectorScript_OwnsDragGesturesOnlyInScreenshotMode(t *testing.T) {
	wants := []string{
		"function onScreenshotPointerDown(",
		"function onScreenshotPointerMove(",
		"function onScreenshotPointerUp(",
		"if (mode !== 'screenshot') return;",
		"send('screenshot-region-selected'",
		"document.addEventListener('pointerdown'",
		"document.removeEventListener('pointerdown'",
	}
	for _, want := range wants {
		if !strings.Contains(inspectorScript, want) {
			t.Errorf("inspector should include %q", want)
		}
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
