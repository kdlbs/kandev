package webapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Runtime serves only validated immutable release files through capability
// URLs. It has no dependency on the ambient Kandev session middleware.
type Runtime struct {
	tokens         *TokenManager
	artifacts      *ArtifactStore
	validate       BindingValidator
	protocol       ProtocolHandler
	frameAncestors []string
}

// ProtocolHandler serves the relative application API after Runtime has
// validated the bearer capability. path is relative to the `_kandev/` root;
// the handler must apply its own response-size and method limits.
type ProtocolHandler func(http.ResponseWriter, *http.Request, string, CapabilityBinding, string)

const runtimeCapabilityPath = "/api/v1/plugins/web-apps/runtime/"

func NewRuntime(tokens *TokenManager, artifacts *ArtifactStore, validate BindingValidator, frameAncestors []string) *Runtime {
	if tokens == nil {
		tokens = NewTokenManager(nil)
	}
	if len(frameAncestors) == 0 {
		frameAncestors = []string{"http://127.0.0.1:38429", "tauri://localhost", "http://tauri.localhost"}
	}
	return &Runtime{tokens: tokens, artifacts: artifacts, validate: validate, frameAncestors: append([]string(nil), frameAncestors...)}
}

// FrameAncestorsForConfig returns the exact browser and desktop shell origins
// that may embed a capability runtime. Ports are explicit because a wildcard
// frame-src/frame-ancestors rule would let an unrelated local service frame a
// canvas. webOrigin is the launcher-provided SPA origin when it differs from
// the backend origin.
func FrameAncestorsForConfig(backendPort, webPort int, webOrigin string) ([]string, error) {
	if backendPort < 1 || backendPort > 65535 || webPort < 1 || webPort > 65535 {
		return nil, fmt.Errorf("webapp: invalid runtime origin port")
	}
	origins := []string{
		fmt.Sprintf("http://localhost:%d", backendPort),
		fmt.Sprintf("http://127.0.0.1:%d", backendPort),
		fmt.Sprintf("http://localhost:%d", webPort),
		fmt.Sprintf("http://127.0.0.1:%d", webPort),
		"tauri://localhost",
		"http://tauri.localhost",
	}
	if strings.TrimSpace(webOrigin) != "" {
		origins = append(origins, webOrigin)
	}
	return normalizeFrameAncestors(origins)
}

// SetProtocolHandler attaches the versioned browser data protocol. It is
// configured during startup before the HTTP server accepts requests.
func (rt *Runtime) SetProtocolHandler(handler ProtocolHandler) {
	if rt == nil {
		return
	}
	rt.protocol = handler
}

// IssueCapabilityPath creates a short-lived capability URL path. The caller
// supplies the already-authorized binding; the path is relative so the host
// can use it from either the browser SPA or the desktop shell without trusting
// an inbound Host header.
func (rt *Runtime) IssueCapabilityPath(binding CapabilityBinding, ttl time.Duration) (string, error) {
	if rt == nil || rt.tokens == nil {
		return "", ErrRuntimeTokenInvalid
	}
	token, err := rt.tokens.Issue(binding, ttl)
	if err != nil {
		return "", err
	}
	return runtimeCapabilityPath + url.PathEscape(token) + "/", nil
}

// Serve handles one capability URL request. path is the path below the
// capability segment; an empty path serves the declared entry document.
func (rt *Runtime) Serve(w http.ResponseWriter, r *http.Request, token, requestPath string) {
	if rt == nil || rt.tokens == nil {
		writeRuntimeError(w, http.StatusNotFound, ErrRuntimeTokenInvalid)
		return
	}
	binding, err := rt.tokens.Validate(token)
	if err != nil {
		writeRuntimeError(w, http.StatusNotFound, err)
		return
	}
	if rt.validate != nil {
		if err := rt.validate(r.Context(), binding); err != nil {
			writeRuntimeError(w, runtimeAuthorizationStatus(err), err)
			return
		}
	}
	if protocolPath, ok := runtimeProtocolPath(requestPath); ok {
		if rt.protocol == nil {
			writeRuntimeError(w, http.StatusNotFound, ErrArtifactUnavailable)
			return
		}
		rt.protocol(w, r, token, binding, protocolPath)
		return
	}
	name, err := runtimeFilePath(requestPath, binding.Entry)
	if err != nil {
		writeRuntimeError(w, http.StatusNotFound, err)
		return
	}
	if rt.artifacts == nil {
		writeRuntimeError(w, http.StatusNotFound, ErrArtifactUnavailable)
		return
	}
	file, err := rt.artifacts.Open(binding.Artifact, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeRuntimeError(w, http.StatusNotFound, ErrArtifactUnavailable)
			return
		}
		writeRuntimeError(w, http.StatusNotFound, err)
		return
	}
	defer func() { _ = file.Close() }()

	policy, err := BuildContentSecurityPolicy(binding.NetworkOrigins, rt.frameAncestors)
	if err != nil {
		writeRuntimeError(w, http.StatusInternalServerError, err)
		return
	}
	setRuntimeHeaders(w, policy, r.Header.Get("Origin"))
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if stat, err := file.Stat(); err == nil {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	}
	_, _ = io.Copy(w, file)
}

// Handler returns a standard-library handler bound to one capability token.
// Backend routing uses this helper when its router already extracted the
// token and the remainder path.
func (rt *Runtime) Handler(token, requestPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rt.Serve(w, r, token, requestPath)
	})
}

func runtimeFilePath(requestPath, entry string) (string, error) {
	entryName, err := normalizePackagePath(strings.TrimPrefix(strings.TrimSpace(entry), "/"), MaxPathBytes)
	if err != nil {
		return "", err
	}
	name := strings.TrimPrefix(strings.TrimSpace(requestPath), "/")
	if name == "" {
		return entryName, nil
	}
	if strings.HasPrefix(name, "_kandev/") || name == "_kandev" {
		return "", ErrUnsafePath
	}
	name, err = normalizePackagePath(name, MaxPathBytes)
	if err != nil {
		return "", err
	}
	// The capability URL is rooted at the application entry directory. This
	// keeps ordinary relative references such as ./app.js and ./app.css
	// working when the manifest entry is nested (for example ui/index.html),
	// while still allowing an explicit package-root path such as ui/app.js.
	entryDir := path.Dir(entryName)
	if entryDir != "." && !strings.HasPrefix(name, entryDir+"/") {
		name = path.Join(entryDir, name)
	}
	return normalizePackagePath(name, MaxPathBytes)
}

func runtimeProtocolPath(requestPath string) (string, bool) {
	name := strings.TrimPrefix(strings.TrimSpace(requestPath), "/")
	if name == "_kandev" {
		return "", true
	}
	if strings.HasPrefix(name, "_kandev/") {
		return strings.TrimPrefix(name, "_kandev/"), true
	}
	return "", false
}

func setRuntimeHeaders(w http.ResponseWriter, policy, origin string) {
	w.Header().Set("Content-Security-Policy", policy)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	if origin == "null" {
		w.Header().Set("Access-Control-Allow-Origin", "null")
		w.Header().Add("Vary", "Origin")
	}
}

// SetProtocolHeaders applies the headers shared by capability API and event
// responses. It is exported for the protocol adapter, while asset responses
// additionally receive the response CSP from setRuntimeHeaders.
func SetProtocolHeaders(w http.ResponseWriter, origin string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	if origin == "null" {
		w.Header().Set("Access-Control-Allow-Origin", "null")
		w.Header().Add("Vary", "Origin")
	}
}

func writeRuntimeError(w http.ResponseWriter, status int, err error) {
	code := runtimeErrorCode(err)
	body, marshalErr := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: code})
	if marshalErr != nil {
		body = []byte(`{"error":"runtime_unavailable"}`)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func runtimeAuthorizationStatus(err error) int {
	if errors.Is(err, ErrRuntimeTokenStale) {
		return http.StatusUnauthorized
	}
	return http.StatusForbidden
}

func runtimeErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrRuntimeTokenInvalid):
		return ErrRuntimeTokenInvalid.Error()
	case errors.Is(err, ErrRuntimeTokenExpired):
		return ErrRuntimeTokenExpired.Error()
	case errors.Is(err, ErrRuntimeTokenStale):
		return ErrRuntimeTokenStale.Error()
	case errors.Is(err, ErrArtifactUnavailable):
		return ErrArtifactUnavailable.Error()
	case errors.Is(err, ErrUnsafePath):
		return ErrUnsafePath.Error()
	default:
		return "runtime_unavailable"
	}
}

// Keep context in this file's API surface so implementations of a validator
// can use request cancellation without importing the HTTP package elsewhere.
var _ BindingValidator = func(context.Context, CapabilityBinding) error { return nil }
