package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func callTUIMCPContext() (string, error) {
	var configPath string
	for index, arg := range os.Args {
		if arg == "--mcp-config" && index+1 < len(os.Args) {
			configPath = os.Args[index+1]
			break
		}
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	var config struct {
		Servers map[string]struct {
			URL string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", err
	}
	url := config.Servers["kandev"].URL
	if url == "" {
		return "", errors.New("missing Kandev MCP URL")
	}
	client, err := mcpclient.NewStreamableHttpClient(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		return "", err
	}
	initialize := mcp.InitializeRequest{}
	initialize.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initialize.Params.ClientInfo = mcp.Implementation{Name: "mock-agent-tui", Version: "1.0"}
	if _, err := client.Initialize(ctx, initialize); err != nil {
		return "", err
	}
	request := mcp.CallToolRequest{}
	request.Params.Name = "list_task_sessions_kandev"
	request.Params.Arguments = map[string]any{"task_id": os.Getenv("KANDEV_TASK_ID")}
	result, err := client.CallTool(ctx, request)
	if err != nil {
		return "", err
	}
	if result.IsError {
		return "", errors.New("remote MCP context request failed")
	}
	return extractMCPResultText(result), nil
}
