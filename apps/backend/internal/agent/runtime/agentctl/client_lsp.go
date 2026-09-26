package client

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
)

// DialLSP opens a raw WebSocket stream to the task-host LSP bridge.
func (c *Client) DialLSP(ctx context.Context, language string, autoInstall bool) (*websocket.Conn, *http.Response, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, nil, err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/api/v1/lsp/stream"
	q := u.Query()
	q.Set("language", language)
	if autoInstall {
		q.Set("autoInstall", "true")
	}
	u.RawQuery = q.Encode()

	return websocket.DefaultDialer.DialContext(ctx, u.String(), c.wsAuthHeaders())
}
