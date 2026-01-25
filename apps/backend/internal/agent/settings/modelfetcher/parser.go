package modelfetcher

import (
	"strings"

	"github.com/kandev/kandev/internal/agent/registry"
)

// ModelParser defines the interface for parsing model output from different agents
type ModelParser interface {
	// Parse parses the raw output from a model listing command into ModelEntry structs
	Parse(output string, defaultModel string) ([]registry.ModelEntry, error)
}

// DefaultParser implements ModelParser with a default parsing strategy
// Expected format: one model ID per line, optionally in "provider/model" format
type DefaultParser struct{}

func (p *DefaultParser) Parse(output string, defaultModel string) ([]registry.ModelEntry, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	models := make([]registry.ModelEntry, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		entry := p.parseLine(line, defaultModel)
		models = append(models, entry)
	}

	return models, nil
}

func (p *DefaultParser) parseLine(line string, defaultModel string) registry.ModelEntry {
	var provider, modelID, name string

	// Check for provider/model format - split on the LAST "/" to get provider and model name
	// Examples:
	//   "opencode/big-pickle" -> provider: "opencode", name: "big-pickle"
	//   "github-copilot/gemini-3-flash" -> provider: "github-copilot", name: "gemini-3-flash"
	//   "openrouter/qwen/qwen3-30b" -> provider: "openrouter/qwen", name: "qwen3-30b"
	if idx := strings.LastIndex(line, "/"); idx > 0 {
		provider = line[:idx]
		modelID = line
		name = line[idx+1:] // Use the model part after last "/" as name
	} else {
		// No provider prefix - use the whole line
		provider = ""
		modelID = line
		name = line
	}

	return registry.ModelEntry{
		ID:        modelID,
		Name:      name,
		Provider:  provider,
		IsDefault: modelID == defaultModel,
		Source:    "dynamic",
	}
}

// OpenCodeParser implements ModelParser for OpenCode agent
// OpenCode outputs models in "provider/model-name" format, one per line
type OpenCodeParser struct {
	DefaultParser
}

// NewOpenCodeParser creates a new OpenCode model parser
func NewOpenCodeParser() *OpenCodeParser {
	return &OpenCodeParser{}
}

// GetParser returns the appropriate parser for an agent
func GetParser(agentID string) ModelParser {
	switch agentID {
	case "opencode":
		return NewOpenCodeParser()
	default:
		return &DefaultParser{}
	}
}
