// Package initialmode materializes the per-session agent configuration
// directory used to give an agent its permission mode before it starts.
package initialmode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Request describes one materialization.
type Request struct {
	// SettingsJSON contains settings from an explicitly selected bundle. Empty
	// input creates a mode-only settings object.
	SettingsJSON []byte
	// TargetDir is the executor-owned per-session configuration directory.
	TargetDir string
	// SettingsFileName is the settings file inside TargetDir.
	SettingsFileName string
	// ModeKeyPath is the nested key path that carries the mode.
	ModeKeyPath []string
	// ModeValue is the agent-specific value to write at that path.
	ModeValue string
}

// Materialize writes a private settings file into an executor-owned session
// directory. It reads no host configuration files.
func Materialize(req Request) (string, error) {
	if req.TargetDir == "" {
		return "", fmt.Errorf("initialmode: target directory is required")
	}
	if req.SettingsFileName == "" || len(req.ModeKeyPath) == 0 {
		return "", fmt.Errorf("initialmode: settings file and mode key path are required")
	}
	settingsJSON, err := MergeSettings(req.SettingsJSON, req.ModeKeyPath, req.ModeValue)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(req.TargetDir, 0o700); err != nil {
		return "", fmt.Errorf("initialmode: create %s: %w", req.TargetDir, err)
	}
	path := filepath.Join(req.TargetDir, req.SettingsFileName)
	if err := writeFileAtomically(path, settingsJSON); err != nil {
		return "", fmt.Errorf("initialmode: write %s: %w", path, err)
	}
	return req.TargetDir, nil
}

// MergeSettings applies a mode to settings bytes received from an explicitly
// selected bundle. A nil input represents no selected settings file.
func MergeSettings(settingsJSON []byte, modeKeyPath []string, modeValue string) ([]byte, error) {
	if len(modeKeyPath) == 0 {
		return nil, fmt.Errorf("initialmode: mode key path is required")
	}
	settings := map[string]any{}
	if len(settingsJSON) > 0 {
		if err := json.Unmarshal(settingsJSON, &settings); err != nil {
			return nil, fmt.Errorf("initialmode: selected settings are not valid JSON: %w", err)
		}
		if settings == nil {
			return nil, fmt.Errorf("initialmode: selected settings root must be a JSON object")
		}
	}
	setNested(settings, modeKeyPath, modeValue)
	encoded, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("initialmode: encode settings: %w", err)
	}
	return append(encoded, '\n'), nil
}

func writeFileAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return nil
}

// setNested assigns value at the nested key path, replacing any non-object it
// has to traverse.
func setNested(root map[string]any, path []string, value string) {
	current := root
	for _, key := range path[:len(path)-1] {
		next, ok := current[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
	current[path[len(path)-1]] = value
}
