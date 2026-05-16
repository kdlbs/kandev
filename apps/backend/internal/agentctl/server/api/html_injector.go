package api

import (
	"bytes"
	_ "embed"
	"io"
	"net/http"
	"strconv"
	"strings"
)

//go:embed scripts/inspector.js
var inspectorScript string

// stripIframeSecurityHeaders removes headers that prevent iframe embedding
// or block injected inline scripts from running.
func stripIframeSecurityHeaders(h http.Header) {
	h.Del("Content-Security-Policy")
	h.Del("Content-Security-Policy-Report-Only")
	h.Del("X-Frame-Options")
}

// injectInspectorScript inserts the inspector <script> tag before </body>.
// Falls back to appending at end of document if </body> is absent.
func injectInspectorScript(html []byte) []byte {
	s := string(html)
	tag := "<script>" + inspectorScript + "</script>"
	pos := strings.LastIndex(strings.ToLower(s), "</body>")
	if pos >= 0 {
		return []byte(s[:pos] + tag + s[pos:])
	}
	return append(html, []byte(tag)...)
}

// injectScriptsIntoResponse modifies an HTML response in-place: strips
// iframe-blocking headers and injects the inspector script. Assumes the
// caller has prevented gzip on the upstream request (Accept-Encoding stripped).
func injectScriptsIntoResponse(resp *http.Response) error {
	stripIframeSecurityHeaders(resp.Header)

	body, err := io.ReadAll(resp.Body)
	if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}

	modified := injectInspectorScript(body)
	resp.Body = io.NopCloser(bytes.NewReader(modified))
	resp.Header.Del("Content-Encoding")
	resp.Header.Set("Content-Length", strconv.Itoa(len(modified)))
	resp.ContentLength = int64(len(modified))
	return nil
}
