package mcpconfig

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

const kandevMCPServerName = "kandev"

var cursorPluginMCPMutex sync.Mutex

type pluginMCPSource struct {
	server  agentctltypes.McpServer
	source  string
	modTime int64
	isUser  bool
}

// DiscoverCursorPluginMCPServers scans Cursor plugin manifests and user-level
// MCP configuration under cursorHome, returning deduplicated candidate servers.
func DiscoverCursorPluginMCPServers(cursorHome string) ([]agentctltypes.McpServer, error) {
	cursorPluginMCPMutex.Lock()
	defer cursorPluginMCPMutex.Unlock()

	if strings.TrimSpace(cursorHome) == "" {
		return nil, nil
	}

	sources, err := collectCursorPluginMCPSources(cursorHome)
	if err != nil {
		return nil, err
	}
	if len(sources) == 0 {
		return nil, nil
	}

	sort.Slice(sources, func(i, j int) bool {
		if sources[i].isUser != sources[j].isUser {
			return sources[i].isUser
		}
		if sources[i].modTime != sources[j].modTime {
			return sources[i].modTime > sources[j].modTime
		}
		return sources[i].source < sources[j].source
	})

	seen := make(map[string]struct{})
	result := make([]agentctltypes.McpServer, 0, len(sources))
	for _, src := range sources {
		name := strings.TrimSpace(src.server.Name)
		if name == "" || strings.EqualFold(name, kandevMCPServerName) {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, src.server)
	}

	return result, nil
}

func collectCursorPluginMCPSources(cursorHome string) ([]pluginMCPSource, error) {
	var sources []pluginMCPSource

	// 1. User-level global configuration (~/.cursor/mcp.json)
	userConfigPath := filepath.Join(cursorHome, "mcp.json")
	if userSources, err := readMCPFileSources(userConfigPath, true); err == nil {
		sources = append(sources, userSources...)
	}

	// 2. Plugins directory
	pluginsDir := filepath.Join(cursorHome, "plugins")
	pluginDirs := discoverPluginDirectories(pluginsDir)
	for _, dir := range pluginDirs {
		dirSources := readPluginDirectoryMCPSources(dir)
		sources = append(sources, dirSources...)
	}

	return sources, nil
}

func discoverPluginDirectories(pluginsRoot string) []string {
	info, err := os.Lstat(pluginsRoot)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil
	}

	var pluginDirs []string
	subRoots := []string{"cache", "local", "marketplaces"}
	for _, sub := range subRoots {
		subPath := filepath.Join(pluginsRoot, sub)
		pluginDirs = append(pluginDirs, scanPluginSubtree(subPath, 0)...)
	}
	return pluginDirs
}

func scanPluginSubtree(root string, depth int) []string {
	if depth > 5 {
		return nil
	}
	info, err := os.Lstat(root)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil
	}

	if isPluginDirectory(root) {
		return []string{root}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var results []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		child := filepath.Join(root, entry.Name())
		results = append(results, scanPluginSubtree(child, depth+1)...)
	}
	return results
}

func isPluginDirectory(dir string) bool {
	candidates := []string{
		"mcp.json",
		".mcp.json",
		filepath.Join(".cursor-plugin", "plugin.json"),
		"plugin.json",
	}
	for _, c := range candidates {
		target := filepath.Join(dir, c)
		if info, err := os.Lstat(target); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func readPluginDirectoryMCPSources(pluginDir string) []pluginMCPSource {
	candidates := []string{
		filepath.Join(pluginDir, "mcp.json"),
		filepath.Join(pluginDir, ".mcp.json"),
		filepath.Join(pluginDir, ".cursor-plugin", "plugin.json"),
		filepath.Join(pluginDir, "plugin.json"),
	}

	for _, path := range candidates {
		sources, err := readMCPFileSources(path, false)
		if err == nil && len(sources) > 0 {
			return sources
		}
	}
	return nil
}

type mcpServersContainer struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

func readMCPFileSources(path string, isUser bool) ([]pluginMCPSource, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, 1024*1024))
	if err != nil {
		return nil, err
	}

	var container mcpServersContainer
	if err := json.Unmarshal(data, &container); err != nil || container.MCPServers == nil {
		return nil, errors.New("invalid mcpServers json")
	}

	var sources []pluginMCPSource
	for name, raw := range container.MCPServers {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" || strings.EqualFold(trimmedName, kandevMCPServerName) {
			continue
		}
		server, ok := parseMCPServerEntry(trimmedName, raw)
		if ok {
			sources = append(sources, pluginMCPSource{
				server:  server,
				source:  path,
				modTime: info.ModTime().UnixNano(),
				isUser:  isUser,
			})
		}
	}

	return sources, nil
}

type rawMCPEntry struct {
	Type        string            `json:"type,omitempty"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Disabled    bool              `json:"disabled,omitempty"`
	AutoApprove []string          `json:"autoApprove,omitempty"`
}

func parseMCPServerEntry(name string, raw json.RawMessage) (agentctltypes.McpServer, bool) {
	var entry rawMCPEntry
	if err := json.Unmarshal(raw, &entry); err != nil || entry.Disabled {
		return agentctltypes.McpServer{}, false
	}

	if entry.URL != "" {
		transport := normalizeNetworkTransport(entry.Type)
		return agentctltypes.McpServer{
			Name:    name,
			Type:    transport,
			URL:     entry.URL,
			Headers: entry.Headers,
		}, true
	}

	if entry.Command != "" {
		return agentctltypes.McpServer{
			Name:    name,
			Type:    string(ServerTypeStdio),
			Command: entry.Command,
			Args:    entry.Args,
			Env:     entry.Env,
		}, true
	}

	return agentctltypes.McpServer{}, false
}

func normalizeNetworkTransport(rawType string) string {
	switch strings.ToLower(strings.TrimSpace(rawType)) {
	case "sse":
		return string(ServerTypeSSE)
	case "streamable_http", "streamable-http":
		return string(ServerTypeStreamableHTTP)
	default:
		return string(ServerTypeHTTP)
	}
}
