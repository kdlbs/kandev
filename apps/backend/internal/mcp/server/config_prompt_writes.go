package mcp

import (
	"context"
	"strings"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (s *Server) registerConfigPromptWriteTools() {
	create := mcp.NewTool("create_shared_prompt_kandev", sharedPromptWriteOptions(
		"Create a saved prompt by name and content. Fails if the exact name exists. Use only when the operator explicitly requested or agreed to create the prompt, then read back the saved result. Create shared prompts before saving workflow steps that reference them.", false)...)
	update := mcp.NewTool("update_shared_prompt_kandev", sharedPromptWriteOptions(
		"Replace a saved prompt's content by exact, case-sensitive name. Any custom prompt can be updated; built-in prompts reject agent writes. This changes every future @name reference. Apply only changes explicitly requested or agreed by the operator, then read back the saved result.", true)...)
	s.mcpServer.AddTool(create, s.wrapHandler(create.Name, s.writeSharedPromptHandler(ws.ActionMCPCreateSharedPrompt)))
	s.mcpServer.AddTool(update, s.wrapHandler(update.Name, s.writeSharedPromptHandler(ws.ActionMCPUpdateSharedPrompt)))
}

func sharedPromptWriteOptions(description string, destructive bool) []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithDescription(description),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(destructive),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("name", mcp.Required(), mcp.MinLength(1), mcp.Description("Exact saved prompt name. Surrounding whitespace is ignored.")),
		mcp.WithString("content", mcp.Required(), mcp.MinLength(1), mcp.Description("Complete replacement content, up to 1 MiB in UTF-8. Surrounding whitespace is trimmed.")),
	}
}

func (s *Server) writeSharedPromptHandler(action string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := strings.TrimSpace(req.GetString("name", ""))
		content := req.GetString("content", "")
		if name == "" || strings.TrimSpace(content) == "" {
			return mcp.NewToolResultError("name and content are required"), nil
		}
		return s.forwardToBackend(ctx, action, map[string]interface{}{"name": name, "content": content})
	}
}
