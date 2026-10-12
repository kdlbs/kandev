package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Long-lived connections outlive the request that authenticated them, so the
// credential check they passed at connect time is the only one they ever get.
// The credential issued by the highest-numbered rotation is the only one that
// authenticates a stream, which means a connection whose credential has been
// superseded must be torn down rather than left reading and writing the
// workspace after every ordinary request from the same holder is refused.
//
// Both helpers take the invalidation channel from the gin context, where the
// authenticating accept check placed it. Calling Invalidated() afresh here
// would reopen a window in which a rotation between the accept check and the
// lookup hands back the channel for the generation that replaced the one just
// authenticated -- a channel that never closes for the rotation that actually
// superseded this connection's credential.

// closeOnCredentialInvalidation closes conn once the credential this request
// authenticated under is superseded. The returned function releases the
// watcher and must be called when the handler returns.
func closeOnCredentialInvalidation(c *gin.Context, conn *websocket.Conn) func() {
	invalidated := credentialInvalidatedFromContext(c)
	if invalidated == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-invalidated:
			_ = conn.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}

// credentialScopedRequest returns c.Request bound to a context that is
// cancelled once the credential it authenticated under is superseded.
// httputil.ReverseProxy closes the upstream connection when the inbound
// request's context is cancelled, including for a connection it has upgraded,
// so a proxied socket terminates with the credential that opened it. The
// returned function releases the watcher and must be called when the handler
// returns.
func credentialScopedRequest(c *gin.Context) (*http.Request, func()) {
	invalidated := credentialInvalidatedFromContext(c)
	if invalidated == nil {
		return c.Request, func() {}
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	go func() {
		select {
		case <-invalidated:
			cancel()
		case <-ctx.Done():
		}
	}()
	return c.Request.WithContext(ctx), cancel
}
