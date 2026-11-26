package antigravityruntime

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
)

const nativeMCPConfigName = "mcp_config.json"

const maxNativeMCPSourceBytes = 1 << 20

// ScanNativeMCPServerNames returns the Antigravity server names that can
// contribute flattened ACP tool aliases. A failed read marks the inventory
// incomplete so callers can keep permission requests pending.
func ScanNativeMCPServerNames(workDir, home string) ([]string, bool) {
	if workDir == "" || home == "" {
		return nil, false
	}
	names := make(map[string]struct{})
	complete := true
	workspaceRoots, rootsComplete := antigravityWorkspaceConfigRoots(workDir)
	complete = complete && rootsComplete
	configRoots := append([]string{filepath.Join(home, ".gemini", "config")}, workspaceRoots...)
	for _, root := range configRoots {
		if hasUnscannedPluginDiscoveryManifest(root) {
			complete = false
		}
		if !nativeConfigRootIsSafe(root) {
			complete = false
			continue
		}
		path := filepath.Join(root, nativeMCPConfigName)
		found, ok := readNativeMCPConfig(path)
		complete = complete && ok
		for _, name := range found {
			names[name] = struct{}{}
		}
	}
	for _, root := range configRoots {
		found, ok := scanNativePluginMCPConfigs(filepath.Join(root, "plugins"))
		complete = complete && ok
		for _, name := range found {
			names[name] = struct{}{}
		}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	slices.Sort(result)
	return result, complete
}

func antigravityWorkspaceConfigRoots(workDir string) ([]string, bool) {
	absolute, err := filepath.Abs(workDir)
	if err != nil {
		return nil, false
	}
	current := filepath.Clean(absolute)
	var ancestors []string
	for {
		ancestors = append(ancestors, current)
		gitPath := filepath.Join(current, ".git")
		if _, err := os.Lstat(gitPath); err == nil {
			return workspaceCustomizationRoots(ancestors), true
		} else if !errors.Is(err, os.ErrNotExist) {
			return workspaceCustomizationRoots(ancestors), false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return workspaceCustomizationRoots(ancestors[:1]), true
		}
		current = parent
	}
}

func workspaceCustomizationRoots(ancestors []string) []string {
	const (
		agentsDir = ".agents"
		agentDir  = ".agent"
		agentsOld = "_agents"
		agentOld  = "_agent"
	)
	rootNames := [...]string{agentsDir, agentDir, agentsOld, agentOld}
	roots := make([]string, 0, len(ancestors)*len(rootNames))
	for _, ancestor := range ancestors {
		for _, name := range rootNames {
			roots = append(roots, filepath.Join(ancestor, name))
		}
	}
	return roots
}

func hasUnscannedPluginDiscoveryManifest(configRoot string) bool {
	_, exists, complete := readNativeMCPSource(filepath.Join(configRoot, "plugins.json"))
	return exists || !complete
}

func nativeConfigRootIsSafe(root string) bool {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

func readNativeMCPConfig(path string) ([]string, bool) {
	data, exists, complete := readNativeMCPSource(path)
	if !exists {
		return nil, true
	}
	if !complete {
		return nil, false
	}
	var config struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, false
	}
	names := make([]string, 0, len(config.Servers))
	for name := range config.Servers {
		names = append(names, name)
	}
	return names, true
}

func scanNativePluginMCPConfigs(root string) ([]string, bool) {
	entries, complete := readNativePluginEntries(root)
	if !complete {
		return nil, false
	}
	names := make([]string, 0)
	for _, entry := range entries {
		servers, ok := scanNativePluginEntry(root, entry)
		complete = complete && ok
		names = append(names, servers...)
	}
	return names, complete
}

func readNativePluginEntries(root string) ([]os.DirEntry, bool) {
	rootInfo, rootErr := os.Lstat(root)
	if errors.Is(rootErr, os.ErrNotExist) {
		return nil, true
	}
	if rootErr != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, false
	}
	entries, err := os.ReadDir(root)
	return entries, err == nil
}

func scanNativePluginEntry(root string, entry os.DirEntry) ([]string, bool) {
	pluginPath := filepath.Join(root, entry.Name())
	info, err := os.Lstat(pluginPath)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return nil, false
	}
	if !info.IsDir() {
		return nil, true
	}
	mcpConfigPath := filepath.Join(pluginPath, nativeMCPConfigName)
	if _, exists, complete := readNativeMCPSource(mcpConfigPath); !exists {
		return nil, complete
	}
	pluginName, ok := readNativePluginName(pluginPath)
	if !ok {
		return nil, false
	}
	servers, ok := readNativeMCPConfig(mcpConfigPath)
	if !ok {
		return nil, false
	}
	names := make([]string, 0, len(servers))
	for _, server := range servers {
		if server != "" {
			names = append(names, pluginName+"_"+server)
		}
	}
	return names, true
}

func readNativePluginName(pluginRoot string) (string, bool) {
	for _, path := range []string{
		filepath.Join(pluginRoot, ".antigravity-plugin", "plugin.json"),
		filepath.Join(pluginRoot, "plugin.json"),
	} {
		data, exists, complete := readNativeMCPSource(path)
		if !exists {
			continue
		}
		if !complete {
			return "", false
		}
		var manifest struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return "", false
		}
		if manifest.Name != "" {
			return manifest.Name, true
		}
		return filepath.Base(pluginRoot), true
	}
	return "", false
}

func readNativeMCPSource(path string) (data []byte, exists, complete bool) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, true
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, true, false
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, true, false
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			data, exists, complete = nil, true, false
		}
	}()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, true, false
	}
	data, err = io.ReadAll(io.LimitReader(file, maxNativeMCPSourceBytes+1))
	if err != nil || len(data) > maxNativeMCPSourceBytes {
		return nil, true, false
	}
	return data, true, true
}
