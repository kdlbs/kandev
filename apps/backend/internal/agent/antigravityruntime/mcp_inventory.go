package antigravityruntime

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
)

const nativeMCPConfigName = "mcp_config.json"

// ScanNativeMCPServerNames returns the Antigravity server names that can
// contribute flattened ACP tool aliases. A failed read marks the inventory
// incomplete so callers can keep permission requests pending.
func ScanNativeMCPServerNames(workDir, home string) ([]string, bool) {
	if workDir == "" || home == "" {
		return nil, false
	}
	names := make(map[string]struct{})
	complete := true
	for _, path := range []string{
		filepath.Join(home, ".gemini", "config", nativeMCPConfigName),
		filepath.Join(workDir, ".agents", nativeMCPConfigName),
	} {
		found, ok := readNativeMCPConfig(path)
		complete = complete && ok
		for _, name := range found {
			names[name] = struct{}{}
		}
	}
	for _, root := range []string{
		filepath.Join(home, ".gemini", "config", "plugins"),
		filepath.Join(workDir, ".agents", "plugins"),
	} {
		found, ok := scanNativePluginMCPConfigs(root)
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

func readNativeMCPConfig(path string) ([]string, bool) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, true
	}
	if err != nil {
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
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, true
	}
	if err != nil {
		return nil, false
	}
	names := make([]string, 0)
	complete := true
	for _, entry := range entries {
		pluginPath := filepath.Join(root, entry.Name())
		info, statErr := os.Stat(pluginPath)
		if statErr != nil {
			complete = false
			continue
		}
		if !info.IsDir() {
			continue
		}
		mcpConfigPath := filepath.Join(pluginPath, nativeMCPConfigName)
		if _, statErr := os.Stat(mcpConfigPath); errors.Is(statErr, os.ErrNotExist) {
			continue
		} else if statErr != nil {
			complete = false
			continue
		}
		pluginName, nameOK := readNativePluginName(pluginPath)
		complete = complete && nameOK
		if !nameOK {
			continue
		}
		servers, ok := readNativeMCPConfig(mcpConfigPath)
		complete = complete && ok
		for _, server := range servers {
			if server != "" {
				names = append(names, pluginName+"_"+server)
			}
		}
	}
	return names, complete
}

func readNativePluginName(pluginRoot string) (string, bool) {
	for _, path := range []string{
		filepath.Join(pluginRoot, ".antigravity-plugin", "plugin.json"),
		filepath.Join(pluginRoot, "plugin.json"),
	} {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
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
