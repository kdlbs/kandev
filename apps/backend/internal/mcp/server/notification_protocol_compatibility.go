package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// streamableHTTPHandler supplements missing notification metadata while leaving
// protocol validation and notification dispatch with the SDK.
func (s *Server) streamableHTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version := r.Header.Get(mcp.HeaderProtocolVersion)
		method := r.Header.Get(mcp.HeaderMethod)
		if r.Method == http.MethodPost && r.Body != nil &&
			(method == "" || strings.HasPrefix(method, "notifications/")) &&
			mcp.IsModernProtocol(version) && s.httpServer.SupportsProtocolVersion(version) {
			// Read lazily so the SDK's transport checks run before the body is consumed.
			r.Body = &notificationBody{original: r.Body, version: version}
			// The supplemented body's length is unknown until the SDK reads it.
			r.ContentLength = -1
		}
		s.httpServer.ServeHTTP(w, r)
	})
}

// notificationBody defers reading and supplementing the request body until the
// SDK first reads it.
type notificationBody struct {
	original io.ReadCloser
	version  string
	reader   io.Reader
}

func (b *notificationBody) Read(p []byte) (int, error) {
	if b.reader == nil {
		body, err := io.ReadAll(b.original)
		if err != nil {
			// Preserve the SDK's read-error response.
			b.reader = notificationBodyReadError{err}
		} else {
			b.reader = bytes.NewReader(supplementNotificationProtocolVersion(body, b.version))
		}
	}
	return b.reader.Read(p)
}

func (b *notificationBody) Close() error {
	return b.original.Close()
}

func supplementNotificationProtocolVersion(body []byte, version string) []byte {
	message, ok := uniqueNotificationObject(body)
	if !ok || !isNotificationEnvelope(body, message) {
		return body
	}
	params := make(map[string]json.RawMessage)
	if raw, present := message["params"]; present {
		params, ok = uniqueNotificationObject(raw)
		if !ok {
			return body
		}
	}
	for key := range params {
		if strings.EqualFold(key, "_meta") {
			return body
		}
	}
	meta, err := json.Marshal(map[string]string{mcp.MetaKeyProtocolVersion: version})
	if err != nil {
		return body
	}
	params["_meta"] = meta
	message["params"], err = json.Marshal(params)
	if err != nil {
		return body
	}
	updated, err := json.Marshal(message)
	if err != nil {
		return body
	}
	return updated
}

// uniqueNotificationObject preserves SDK interpretation of duplicate members by
// declining to rewrite objects that would lose a member during map decoding.
func uniqueNotificationObject(body []byte) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, false
	}
	object := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err = decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return nil, false
		}
		if _, duplicate := object[key]; duplicate {
			return nil, false
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, false
		}
		object[key] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, false
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, false
	}
	return object, true
}

func isNotificationEnvelope(body []byte, message map[string]json.RawMessage) bool {
	// The SDK's struct decoding also recognizes differently cased field names.
	// Leave those messages untouched rather than creating competing fields.
	for key := range message {
		for _, core := range []string{"id", "jsonrpc", "method", "params"} {
			if strings.EqualFold(key, core) && (core == "id" || key != core) {
				return false
			}
		}
	}
	var envelope struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
	}
	return json.Unmarshal(body, &envelope) == nil && envelope.JSONRPC == mcp.JSONRPC_VERSION && envelope.Method != ""
}

type notificationBodyReadError struct {
	err error
}

func (r notificationBodyReadError) Read([]byte) (int, error) {
	return 0, r.err
}

// ServeStreamableHTTP serves the existing MCP transport behind an authenticated host route.
func (s *Server) ServeStreamableHTTP(w http.ResponseWriter, r *http.Request) {
	s.streamableHTTPHandler().ServeHTTP(w, r)
}
