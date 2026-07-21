package websocket

import (
	"net/http"

	"github.com/kandev/kandev/internal/common/httpmw"
)

// checkWebSocketOrigin validates the Origin header on WebSocket upgrade
// requests to prevent cross-site WebSocket hijacking: without it, any web
// page the user visits could open a socket to the local backend and drive
// session/shell actions on the host.
//
// Requests without an Origin header are allowed (non-browser clients such as
// the CLI or curl); everything else defers to the shared origin trust policy
// in httpmw.AllowedOrigin, the same policy the CORS middleware enforces.
func checkWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return httpmw.AllowedOrigin(origin, r.Host)
}
