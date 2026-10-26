package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	askQuestionToolPrefix    = "ask_user_question"
	askQuestionKeepAliveText = "Waiting for your response in Kandev"
	toolKeepAliveText        = "Kandev is still working on this call"
)

// keepAliveMiddleware streams progress notifications for the lifetime of every
// Kandev tool call, so an agent's MCP client does not abort a call that waits
// for a detached backend or a human answer. The pinger stops before the result
// is handed back to the transport.
func (s *Server) keepAliveMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			emitKeepAlivePings(ctx, stop, toolKeepAliveInterval, s.toolKeepAlive(ctx, req))
		}()
		defer func() {
			close(stop)
			<-done
		}()
		return next(ctx, req)
	}
}

// toolKeepAlive builds the callback that sends one notifications/progress
// message. It uses the client's progress token when present, otherwise a
// per-call token. It is a no-op when no MCP server is bound to the context.
func (s *Server) toolKeepAlive(ctx context.Context, req mcp.CallToolRequest) func() {
	srv := server.ServerFromContext(ctx)
	if srv == nil {
		return func() {}
	}
	var token mcp.ProgressToken
	if req.Params.Meta != nil && req.Params.Meta.ProgressToken != nil {
		token = req.Params.Meta.ProgressToken
	} else {
		token = fmt.Sprintf("kandev:%s:%d", s.sessionID, atomic.AddUint64(&s.keepAliveCalls, 1))
	}
	message := toolKeepAliveText
	if strings.HasPrefix(req.Params.Name, askQuestionToolPrefix) {
		message = askQuestionKeepAliveText
	}
	var progress float64
	return func() {
		progress++
		sendCtx, cancel := context.WithTimeout(ctx, toolKeepAliveSendTimeout)
		defer cancel()
		_ = srv.SendNotificationToClient(sendCtx, "notifications/progress", map[string]any{
			"progressToken": token,
			"progress":      progress,
			messageArg:      message,
		})
	}
}
