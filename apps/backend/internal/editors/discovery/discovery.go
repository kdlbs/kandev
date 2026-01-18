package discovery

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed editors.json
var editorsFS embed.FS

type EditorDefinition struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Scheme  string `json:"scheme"`
	Enabled bool   `json:"enabled"`
}

type Config struct {
	Editors []EditorDefinition `json:"editors"`
}

func LoadDefaults() ([]EditorDefinition, error) {
	data, err := editorsFS.ReadFile("editors.json")
	if err != nil {
		return nil, fmt.Errorf("read editors config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse editors config: %w", err)
	}
	return cfg.Editors, nil
}
