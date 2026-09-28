package mcpconfig

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

const kandevMCPServerName = "kandev"

var cursorPluginMCPMutex sync.Mutex

var unresolvedVarRegex = regexp.MustCompile(`\$\{[^}]+\}`)

// SourceKind identifies the origin of an MCP server definition.
type SourceKind int

const (
	SourceKindPlugin SourceKind = iota
	SourceKindGlobal
	SourceKindProject
	SourceKindProfile
)

// DiscoveredServerCandidate represents a parsed MCP server with full provenance metadata.
type DiscoveredServerCandidate struct {
	Name       string
	Type       ServerType
	SourceKind SourceKind
	PluginName string
	PluginRoot string
	SourcePath string
	ModTime    int64
	Server     agentctltypes.McpServer
}

// ToServerDef converts a candidate to ServerDef for policy resolution.
func (c DiscoveredServerCandidate) ToServerDef() ServerDef {
	return ServerDef{
		Type:    c.Type,
		Command: c.Server.Command,
		Args:    append([]string(nil), c.Server.Args...),
		Env:     cloneStringMap(c.Server.Env),
		URL:     c.Server.URL,
		Headers: cloneStringMap(c.Server.Headers),
	}
}

// DiscoverCursorPluginMCPServers scans Cursor plugin manifests and user-level
// MCP configuration under cursorHome, returning deduplicated candidate servers.
func DiscoverCursorPluginMCPServers(cursorHome string) ([]agentctltypes.McpServer, error) {
	candidates, err := DiscoverCursorPluginCandidates(cursorHome)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{})
	result := make([]agentctltypes.McpServer, 0, len(candidates))
	for _, cand := range candidates {
		name := strings.TrimSpace(cand.Name)
		if name == "" || strings.EqualFold(name, kandevMCPServerName) {
			continue
		}
		lowerName := strings.ToLower(name)
		if _, exists := seen[lowerName]; exists {
			continue
		}
		seen[lowerName] = struct{}{}
		result = append(result, cand.Server)
	}

	return result, nil
}

// DiscoverCursorPluginCandidates returns discovered candidate servers with full provenance.
func DiscoverCursorPluginCandidates(cursorHome string) ([]DiscoveredServerCandidate, error) {
	cursorPluginMCPMutex.Lock()
	defer cursorPluginMCPMutex.Unlock()

	if strings.TrimSpace(cursorHome) == "" {
		return nil, nil
	}

	candidates, err := collectCursorPluginCandidates(cursorHome)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].SourceKind != candidates[j].SourceKind {
			return candidates[i].SourceKind > candidates[j].SourceKind
		}
		if candidates[i].ModTime != candidates[j].ModTime {
			return candidates[i].ModTime > candidates[j].ModTime
		}
		return candidates[i].SourcePath < candidates[j].SourcePath
	})

	return candidates, nil
}

func collectCursorPluginCandidates(cursorHome string) ([]DiscoveredServerCandidate, error) {
	var candidates []DiscoveredServerCandidate

	// 1. User-level global configuration (~/.cursor/mcp.json)
	userConfigPath := filepath.Join(cursorHome, "mcp.json")
	if userCandidates, err := readMCPFileCandidates(userConfigPath, SourceKindGlobal, "", ""); err == nil {
		candidates = append(candidates, userCandidates...)
	}

	// 2. Plugins directory
	pluginsDir := filepath.Join(cursorHome, "plugins")
	pluginRoots := enumerateEligiblePluginRoots(pluginsDir)
	for _, root := range pluginRoots {
		rootCandidates := readEligiblePluginRootCandidates(root)
		candidates = append(candidates, rootCandidates...)
	}

	return candidates, nil
}

type eligiblePluginRoot struct {
	name    string
	path    string
	modTime int64
}

func enumerateEligiblePluginRoots(pluginsDir string) []eligiblePluginRoot {
	info, err := os.Lstat(pluginsDir)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil
	}

	installedMap := readInstalledPluginsRegistry(pluginsDir)
	eligible := enumerateLocalPluginRoots(pluginsDir, installedMap)
	versioned := enumerateVersionedPluginRoots(pluginsDir, installedMap)
	return append(eligible, versioned...)
}

func enumerateLocalPluginRoots(pluginsDir string, installedMap map[string]bool) []eligiblePluginRoot {
	localDir := filepath.Join(pluginsDir, "local")
	localEntries, err := os.ReadDir(localDir)
	if err != nil {
		return nil
	}

	var eligible []eligiblePluginRoot
	for _, entry := range localEntries {
		if !entry.IsDir() {
			continue
		}
		pDir := filepath.Join(localDir, entry.Name())
		if !isInstalledAndEnabled(installedMap, entry.Name(), pDir) {
			continue
		}
		dInfo, err := os.Lstat(pDir)
		if err == nil && dInfo.IsDir() {
			eligible = append(eligible, eligiblePluginRoot{
				name:    entry.Name(),
				path:    pDir,
				modTime: dInfo.ModTime().UnixNano(),
			})
		}
	}
	return eligible
}

func enumerateVersionedPluginRoots(pluginsDir string, installedMap map[string]bool) []eligiblePluginRoot {
	versionedRoots := []string{
		filepath.Join(pluginsDir, "cache"),
		filepath.Join(pluginsDir, "marketplaces"),
	}

	newestByPlugin := make(map[string]eligiblePluginRoot)
	for _, subRoot := range versionedRoots {
		subInfo, err := os.Lstat(subRoot)
		if err != nil || subInfo.Mode()&os.ModeSymlink != 0 || !subInfo.IsDir() {
			continue
		}
		discovered := scanVersionedPluginRoots(subRoot, 0)
		for _, cand := range discovered {
			if !isInstalledAndEnabled(installedMap, cand.name, cand.path) {
				continue
			}
			existing, exists := newestByPlugin[cand.name]
			if !exists || cand.modTime > existing.modTime {
				newestByPlugin[cand.name] = cand
			}
		}
	}

	var eligible []eligiblePluginRoot
	for _, p := range newestByPlugin {
		eligible = append(eligible, p)
	}
	return eligible
}

func scanVersionedPluginRoots(dir string, depth int) []eligiblePluginRoot {
	if depth > 4 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var results []eligiblePluginRoot
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		child := filepath.Join(dir, entry.Name())
		if isCandidatePluginVersionDirectory(child) {
			dInfo, err := os.Lstat(child)
			if err == nil {
				pluginName := derivePluginNameFromPath(child)
				results = append(results, eligiblePluginRoot{
					name:    pluginName,
					path:    child,
					modTime: dInfo.ModTime().UnixNano(),
				})
			}
			continue
		}
		results = append(results, scanVersionedPluginRoots(child, depth+1)...)
	}
	return results
}

func derivePluginNameFromPath(dir string) string {
	parent := filepath.Dir(dir)
	base := filepath.Base(parent)
	if base != "." && base != "/" && base != "cache" && base != "marketplaces" && base != "local" {
		return base
	}
	return filepath.Base(dir)
}

func isCandidatePluginVersionDirectory(dir string) bool {
	// Must contain a manifest
	candidates := []string{
		filepath.Join(".cursor-plugin", "plugin.json"),
		"plugin.json",
		".mcp.json",
		"mcp.json",
	}
	hasManifest := false
	for _, c := range candidates {
		target := filepath.Join(dir, c)
		if info, err := os.Lstat(target); err == nil && info.Mode().IsRegular() {
			hasManifest = true
			break
		}
	}
	if !hasManifest {
		return false
	}

	// Must not be an uninstalled/staging/incomplete folder
	base := strings.ToLower(filepath.Base(dir))
	if base == "_staging" || strings.Contains(base, "uninstalled") || strings.Contains(base, "disabled") {
		return false
	}
	parentBase := strings.ToLower(filepath.Base(filepath.Dir(dir)))
	if parentBase == "uninstalled" || parentBase == "disabled" {
		return false
	}

	return true
}

type pluginRegistryEntry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Path     string `json:"path"`
	Enabled  *bool  `json:"enabled"`
	Disabled bool   `json:"disabled"`
}

func readInstalledPluginsRegistry(pluginsDir string) map[string]bool {
	registryFiles := []string{
		filepath.Join(pluginsDir, "installed.json"),
		filepath.Join(pluginsDir, "plugins.json"),
	}

	for _, rFile := range registryFiles {
		data, err := os.ReadFile(rFile)
		if err != nil {
			continue
		}
		if res := parsePluginRegistryData(data); res != nil {
			return res
		}
	}
	return nil
}

func parsePluginRegistryData(data []byte) map[string]bool {
	if res, ok := tryParseRegistryList(data); ok {
		return res
	}
	if res, ok := tryParseRegistryMap(data); ok {
		return res
	}
	if res, ok := tryParseRegistryArray(data); ok {
		return res
	}
	return nil
}

func tryParseRegistryList(data []byte) (map[string]bool, bool) {
	var obj struct {
		Plugins   []pluginRegistryEntry `json:"plugins"`
		Installed []pluginRegistryEntry `json:"installed"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, false
	}
	if len(obj.Plugins) == 0 && len(obj.Installed) == 0 {
		return nil, false
	}
	result := make(map[string]bool)
	for _, entry := range obj.Plugins {
		addRegistryEntry(result, entry)
	}
	for _, entry := range obj.Installed {
		addRegistryEntry(result, entry)
	}
	return result, true
}

func tryParseRegistryMap(data []byte) (map[string]bool, bool) {
	var mapShape map[string]json.RawMessage
	if err := json.Unmarshal(data, &mapShape); err != nil || len(mapShape) == 0 {
		return nil, false
	}
	result := make(map[string]bool)
	for k, raw := range mapShape {
		var b bool
		if err := json.Unmarshal(raw, &b); err == nil {
			result[strings.ToLower(k)] = b
			continue
		}
		var entry pluginRegistryEntry
		if err := json.Unmarshal(raw, &entry); err == nil {
			if entry.ID == "" && entry.Name == "" {
				entry.ID = k
			}
			addRegistryEntry(result, entry)
		}
	}
	return result, true
}

func tryParseRegistryArray(data []byte) (map[string]bool, bool) {
	var arrayShape []pluginRegistryEntry
	if err := json.Unmarshal(data, &arrayShape); err != nil || len(arrayShape) == 0 {
		return nil, false
	}
	result := make(map[string]bool)
	for _, entry := range arrayShape {
		addRegistryEntry(result, entry)
	}
	return result, true
}

func addRegistryEntry(result map[string]bool, entry pluginRegistryEntry) {
	key := entry.ID
	if key == "" {
		key = entry.Name
	}
	if key == "" {
		return
	}
	enabled := true
	if entry.Enabled != nil {
		enabled = *entry.Enabled
	} else if entry.Disabled {
		enabled = false
	}
	result[strings.ToLower(key)] = enabled
}

func isInstalledAndEnabled(registry map[string]bool, pluginName, pluginPath string) bool {
	if registry == nil {
		// When no central registry exists, completed plugin directories are accepted
		return true
	}
	key := strings.ToLower(pluginName)
	if enabled, exists := registry[key]; exists {
		return enabled
	}
	// Check by directory base name
	pathKey := strings.ToLower(filepath.Base(pluginPath))
	if enabled, exists := registry[pathKey]; exists {
		return enabled
	}
	return false
}

func readEligiblePluginRootCandidates(pluginRoot eligiblePluginRoot) []DiscoveredServerCandidate {
	manifestCandidates := []string{
		filepath.Join(pluginRoot.path, ".cursor-plugin", "plugin.json"),
		filepath.Join(pluginRoot.path, "plugin.json"),
		filepath.Join(pluginRoot.path, ".mcp.json"),
		filepath.Join(pluginRoot.path, "mcp.json"),
	}

	for _, mPath := range manifestCandidates {
		candidates, err := readMCPFileCandidates(mPath, SourceKindPlugin, pluginRoot.name, pluginRoot.path)
		if err == nil && len(candidates) > 0 {
			return candidates
		}
	}
	return nil
}

type pluginDescriptor struct {
	Name       string          `json:"name"`
	Disabled   bool            `json:"disabled"`
	MCPServers json.RawMessage `json:"mcpServers"`
}

type standaloneMCPContainer struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

func readMCPFileCandidates(
	path string,
	kind SourceKind,
	pluginName, pluginRoot string,
) ([]DiscoveredServerCandidate, error) {
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

	modTime := info.ModTime().UnixNano()

	if candidates, handled, err := tryParseDescriptorCandidates(data, path, modTime, kind, pluginName, pluginRoot); handled {
		return candidates, err
	}
	if candidates, ok := tryParseStandaloneCandidates(data, path, modTime, kind, pluginName, pluginRoot); ok {
		return candidates, nil
	}
	if candidates, ok := tryParseBareCandidates(data, path, modTime, kind, pluginName, pluginRoot); ok {
		return candidates, nil
	}

	return nil, errors.New("no valid MCP servers found")
}

func tryParseDescriptorCandidates(
	data []byte,
	path string,
	modTime int64,
	kind SourceKind,
	pluginName, pluginRoot string,
) ([]DiscoveredServerCandidate, bool, error) {
	var desc pluginDescriptor
	if err := json.Unmarshal(data, &desc); err != nil || (len(desc.MCPServers) == 0 && desc.Name == "") {
		return nil, false, nil
	}
	if desc.Disabled {
		return nil, true, errors.New("plugin descriptor is disabled")
	}
	if len(desc.MCPServers) == 0 {
		return nil, false, nil
	}

	var refPath string
	if err := json.Unmarshal(desc.MCPServers, &refPath); err == nil && refPath != "" {
		res, err := resolveDescriptorFileReference(refPath, pluginRoot, kind, pluginName)
		return res, true, err
	}

	var serverMap map[string]json.RawMessage
	if err := json.Unmarshal(desc.MCPServers, &serverMap); err == nil {
		return parseRawServerMap(serverMap, path, modTime, kind, pluginName, pluginRoot), true, nil
	}
	return nil, false, nil
}

func tryParseStandaloneCandidates(
	data []byte,
	path string,
	modTime int64,
	kind SourceKind,
	pluginName, pluginRoot string,
) ([]DiscoveredServerCandidate, bool) {
	var container standaloneMCPContainer
	if err := json.Unmarshal(data, &container); err == nil && container.MCPServers != nil {
		return parseRawServerMap(container.MCPServers, path, modTime, kind, pluginName, pluginRoot), true
	}
	return nil, false
}

func tryParseBareCandidates(
	data []byte,
	path string,
	modTime int64,
	kind SourceKind,
	pluginName, pluginRoot string,
) ([]DiscoveredServerCandidate, bool) {
	var bareMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &bareMap); err == nil && len(bareMap) > 0 {
		candidates := parseRawServerMap(bareMap, path, modTime, kind, pluginName, pluginRoot)
		if len(candidates) > 0 {
			return candidates, true
		}
	}
	return nil, false
}

func resolveDescriptorFileReference(
	relPath, pluginRoot string,
	kind SourceKind,
	pluginName string,
) ([]DiscoveredServerCandidate, error) {
	if filepath.IsAbs(relPath) || strings.Contains(relPath, "..") {
		return nil, errors.New("referenced MCP file path cannot be absolute or traverse directories")
	}

	target := filepath.Join(pluginRoot, filepath.Clean(relPath))

	// Enforce containment after symlink evaluation
	canonicalRoot, err := filepath.EvalSymlinks(pluginRoot)
	if err != nil {
		canonicalRoot = filepath.Clean(pluginRoot)
	}
	canonicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		canonicalTarget = filepath.Clean(target)
	}

	rel, err := filepath.Rel(canonicalRoot, canonicalTarget)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return nil, errors.New("referenced MCP file escapes plugin root")
	}

	return readMCPFileCandidates(target, kind, pluginName, pluginRoot)
}

func parseRawServerMap(
	rawMap map[string]json.RawMessage,
	sourcePath string,
	modTime int64,
	kind SourceKind,
	pluginName, pluginRoot string,
) []DiscoveredServerCandidate {
	var candidates []DiscoveredServerCandidate
	for name, raw := range rawMap {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" || strings.EqualFold(trimmedName, kandevMCPServerName) {
			continue
		}
		cand, ok := parseSingleMCPServerCandidate(trimmedName, raw, sourcePath, modTime, kind, pluginName, pluginRoot)
		if ok {
			candidates = append(candidates, cand)
		}
	}
	return candidates
}

type rawMCPEntry struct {
	Type        string            `json:"type,omitempty"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Disabled    bool              `json:"disabled,omitempty"`
	CWD         string            `json:"cwd,omitempty"`
	AutoApprove []string          `json:"autoApprove,omitempty"`
}

func parseSingleMCPServerCandidate(
	name string,
	raw json.RawMessage,
	sourcePath string,
	modTime int64,
	kind SourceKind,
	pluginName, pluginRoot string,
) (DiscoveredServerCandidate, bool) {
	var entry rawMCPEntry
	if err := json.Unmarshal(raw, &entry); err != nil || entry.Disabled {
		return DiscoveredServerCandidate{}, false
	}
	if strings.TrimSpace(entry.CWD) != "" {
		return DiscoveredServerCandidate{}, false
	}
	if strings.TrimSpace(entry.Command) != "" && strings.TrimSpace(entry.URL) != "" {
		return DiscoveredServerCandidate{}, false
	}

	if strings.TrimSpace(entry.URL) != "" {
		return parseNetworkCandidate(name, entry, sourcePath, modTime, kind, pluginName, pluginRoot)
	}
	if strings.TrimSpace(entry.Command) != "" {
		return parseStdioCandidate(name, entry, sourcePath, modTime, kind, pluginName, pluginRoot)
	}
	return DiscoveredServerCandidate{}, false
}

func parseNetworkCandidate(
	name string,
	entry rawMCPEntry,
	sourcePath string,
	modTime int64,
	kind SourceKind,
	pluginName, pluginRoot string,
) (DiscoveredServerCandidate, bool) {
	rawType := strings.ToLower(strings.TrimSpace(entry.Type))
	if rawType == "stdio" {
		return DiscoveredServerCandidate{}, false
	}
	transport, ok := parseNetworkTransport(rawType)
	if !ok {
		return DiscoveredServerCandidate{}, false
	}
	expandedURL, ok := expandPluginVariables(entry.URL, pluginRoot)
	if !ok {
		return DiscoveredServerCandidate{}, false
	}
	if _, err := url.ParseRequestURI(expandedURL); err != nil {
		return DiscoveredServerCandidate{}, false
	}

	headers, ok := expandHeadersMap(entry.Headers, pluginRoot)
	if !ok {
		return DiscoveredServerCandidate{}, false
	}

	return DiscoveredServerCandidate{
		Name:       name,
		Type:       transport,
		SourceKind: kind,
		PluginName: pluginName,
		PluginRoot: pluginRoot,
		SourcePath: sourcePath,
		ModTime:    modTime,
		Server: agentctltypes.McpServer{
			Name:    name,
			Type:    string(transport),
			URL:     expandedURL,
			Headers: headers,
		},
	}, true
}

func parseStdioCandidate(
	name string,
	entry rawMCPEntry,
	sourcePath string,
	modTime int64,
	kind SourceKind,
	pluginName, pluginRoot string,
) (DiscoveredServerCandidate, bool) {
	rawType := strings.ToLower(strings.TrimSpace(entry.Type))
	if rawType != "" && rawType != "stdio" {
		return DiscoveredServerCandidate{}, false
	}
	expandedCmd, ok := expandPluginVariables(entry.Command, pluginRoot)
	if !ok {
		return DiscoveredServerCandidate{}, false
	}

	expandedArgs, ok := expandStringSlice(entry.Args, pluginRoot)
	if !ok {
		return DiscoveredServerCandidate{}, false
	}

	expandedEnv, ok := expandEnvMap(entry.Env, pluginRoot)
	if !ok {
		return DiscoveredServerCandidate{}, false
	}

	return DiscoveredServerCandidate{
		Name:       name,
		Type:       ServerTypeStdio,
		SourceKind: kind,
		PluginName: pluginName,
		PluginRoot: pluginRoot,
		SourcePath: sourcePath,
		ModTime:    modTime,
		Server: agentctltypes.McpServer{
			Name:    name,
			Type:    string(ServerTypeStdio),
			Command: expandedCmd,
			Args:    expandedArgs,
			Env:     expandedEnv,
		},
	}, true
}

func parseNetworkTransport(rawType string) (ServerType, bool) {
	switch strings.ToLower(strings.TrimSpace(rawType)) {
	case "sse":
		return ServerTypeSSE, true
	case "streamable_http", "streamable-http":
		return ServerTypeStreamableHTTP, true
	case "http", "":
		return ServerTypeHTTP, true
	default:
		return "", false
	}
}

func expandPluginVariables(s, pluginRoot string) (string, bool) {
	if s == "" {
		return "", true
	}
	expanded := s
	if pluginRoot != "" {
		expanded = strings.ReplaceAll(expanded, "${CURSOR_PLUGIN_ROOT}", pluginRoot)
		expanded = strings.ReplaceAll(expanded, "${PLUGIN_ROOT}", pluginRoot)
	}
	// Check for any remaining unresolved variables ${...}
	if unresolvedVarRegex.MatchString(expanded) {
		return "", false
	}
	return expanded, true
}

func expandStringSlice(slice []string, pluginRoot string) ([]string, bool) {
	if slice == nil {
		return nil, true
	}
	result := make([]string, len(slice))
	for i, item := range slice {
		expanded, ok := expandPluginVariables(item, pluginRoot)
		if !ok {
			return nil, false
		}
		result[i] = expanded
	}
	return result, true
}

func expandEnvMap(env map[string]string, pluginRoot string) (map[string]string, bool) {
	if env == nil {
		return nil, true
	}
	result := make(map[string]string, len(env))
	for k, v := range env {
		expandedK, ok := expandPluginVariables(k, pluginRoot)
		if !ok {
			return nil, false
		}
		expandedV, ok := expandPluginVariables(v, pluginRoot)
		if !ok {
			return nil, false
		}
		result[expandedK] = expandedV
	}
	return result, true
}

func expandHeadersMap(headers map[string]string, pluginRoot string) (map[string]string, bool) {
	return expandEnvMap(headers, pluginRoot)
}
