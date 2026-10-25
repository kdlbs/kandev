package sandboxmcp

import (
	"context"
	"io"
	"net/http"
	"strings"
)

func (r *Relay) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Connection", "close")
	if !r.accepts(request) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, request.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	ctx, cancel := context.WithCancel(request.Context())
	stop := context.AfterFunc(r.ctx, cancel)
	defer stop()
	defer cancel()
	response, err := r.forward(ctx, request.Method, requestHeaders(request.Header), body)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	if response == nil || response.Body == nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 500 || (response.StatusCode >= 300 && response.StatusCode < 400) {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	for _, key := range []string{"Content-Type", "Mcp-Session-Id", "MCP-Protocol-Version"} {
		if value := response.Header.Get(key); value != "" {
			w.Header().Set(key, value)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(response.StatusCode)
	stream(w, response.Body)
}

func (r *Relay) accepts(request *http.Request) bool {
	if r.ctx.Err() != nil || request.URL.EscapedPath() != r.path || request.URL.RawQuery != "" || request.URL.ForceQuery || request.Host != r.address || request.Header.Get("Origin") != "" || request.Header.Get("Upgrade") != "" {
		return false
	}
	switch request.Method {
	case http.MethodPost:
		return true
	case http.MethodGet, http.MethodDelete:
		return request.ContentLength == 0 && len(request.TransferEncoding) == 0
	default:
		return false
	}
}

func requestHeaders(input http.Header) http.Header {
	output := make(http.Header)
	for _, key := range []string{"Content-Type", "Accept", "Mcp-Session-Id", "MCP-Protocol-Version", "Last-Event-Id", "Mcp-Method"} {
		if value := input.Get(key); value != "" {
			output.Set(key, value)
		}
	}
	return output
}

func stream(w http.ResponseWriter, body io.Reader) {
	reader := io.LimitReader(body, 64<<20)
	buffer := make([]byte, 32<<10)
	controller := http.NewResponseController(w)
	flush := strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream")
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				return
			}
			if flush && controller.Flush() != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
