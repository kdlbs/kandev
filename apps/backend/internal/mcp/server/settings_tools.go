package mcp

import (
	"context"
	"encoding/json"
	"strings"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// The compact settings tools intentionally advertise only the stable envelope.
// Domain fields arrive through describe_setting_kandev and are validated by
// the backend registry selected for the exact target.
func (s *Server) registerConfigSettingsTools() {
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema(
			"search_settings_kandev",
			"Search setting definitions by metadata. Search never reads saved values or secrets.",
			json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"query":{"type":"string","maxLength":256},"domain":{"type":"string"},"resource_type":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":50},"cursor":{"type":"string"}}}`),
		),
		s.wrapHandler("search_settings_kandev", s.searchSettingsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema(
			"describe_setting_kandev",
			"Describe one setting definition, including its target rules, schema, authority, and supported operations.",
			json.RawMessage(`{"type":"object","additionalProperties":false,"required":["key"],"properties":{"key":{"type":"string","minLength":1},"target":{"type":"object","additionalProperties":false,"properties":{"resource_type":{"type":"string"},"resource_id":{"type":"string"},"workspace_id":{"type":"string"}}},"context":{"type":"object"},"operation":{"type":"string","enum":["read","update","create"]},"include_resource_schema":{"type":"boolean"}}}`),
		),
		s.wrapHandler("describe_setting_kandev", s.describeSettingHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema(
			"get_settings_kandev",
			"Read saved settings for one exact authorized target. Sensitive values are redacted or returned as references.",
			json.RawMessage(`{"type":"object","additionalProperties":false,"required":["target","keys"],"properties":{"target":{"type":"object","additionalProperties":false,"required":["resource_type"],"properties":{"resource_type":{"type":"string"},"resource_id":{"type":"string"},"workspace_id":{"type":"string"}}},"keys":{"type":"array","minItems":1,"maxItems":50,"items":{"type":"string","minLength":1}}}}`),
		),
		s.wrapSensitiveHandler("get_settings_kandev", s.getSettingsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema(
			"update_settings_kandev",
			"Update declared settings for one exact authorized target. Changes are validated by the owning domain before one mutation.",
			json.RawMessage(`{"type":"object","additionalProperties":false,"required":["target","changes"],"properties":{"target":{"type":"object","additionalProperties":false,"required":["resource_type"],"properties":{"resource_type":{"type":"string"},"resource_id":{"type":"string"},"workspace_id":{"type":"string"}}},"changes":{"type":"object","minProperties":1,"maxProperties":50,"additionalProperties":true},"options":{"type":"object"}}}`),
		),
		s.wrapSensitiveHandler("update_settings_kandev", s.updateSettingsHandler()),
	)
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema(
			"list_settings_resources_kandev",
			"List bounded, authorized targets for one registered settings resource type.",
			json.RawMessage(`{"type":"object","additionalProperties":false,"required":["resource_type"],"properties":{"resource_type":{"type":"string","minLength":1},"workspace_id":{"type":"string"},"query":{"type":"string","maxLength":256},"limit":{"type":"integer","minimum":1,"maximum":50},"cursor":{"type":"string"}}}`),
		),
		s.wrapHandler("list_settings_resources_kandev", s.listSettingsResourcesHandler()),
	)
}

func (s *Server) searchSettingsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return s.forwardToBackend(ctx, ws.ActionMCPSearchSettings, cloneArguments(req.GetArguments()))
	}
}

func (s *Server) describeSettingHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		key, err := req.RequireString("key")
		if err != nil || strings.TrimSpace(key) == "" {
			return mcp.NewToolResultError("key is required"), nil
		}
		return s.forwardToBackend(ctx, ws.ActionMCPDescribeSetting, cloneArguments(req.GetArguments()))
	}
}

func (s *Server) getSettingsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if req.GetArguments()["target"] == nil {
			return mcp.NewToolResultError("target is required"), nil
		}
		if req.GetArguments()["keys"] == nil {
			return mcp.NewToolResultError("keys is required"), nil
		}
		return s.forwardToBackend(ctx, ws.ActionMCPGetSettings, cloneArguments(req.GetArguments()))
	}
}

func (s *Server) updateSettingsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		if args["target"] == nil {
			return mcp.NewToolResultError("target is required"), nil
		}
		if args["changes"] == nil {
			return mcp.NewToolResultError("changes is required"), nil
		}
		return s.forwardToBackend(ctx, ws.ActionMCPUpdateSettings, cloneArguments(args))
	}
}

func (s *Server) listSettingsResourcesHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resourceType, err := req.RequireString("resource_type")
		if err != nil || strings.TrimSpace(resourceType) == "" {
			return mcp.NewToolResultError("resource_type is required"), nil
		}
		return s.forwardToBackend(ctx, ws.ActionMCPListSettingsResources, cloneArguments(req.GetArguments()))
	}
}

func cloneArguments(arguments map[string]any) map[string]any {
	if arguments == nil {
		return nil
	}
	cloned := make(map[string]any, len(arguments))
	for key, value := range arguments {
		cloned[key] = value
	}
	return cloned
}
