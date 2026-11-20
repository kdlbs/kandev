package mcpconfig

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
)

const maxMCPSetupServers = 1024
const maxMCPSetupBytes = 256 * 1024

type MCPSetupServer struct {
	DefinitionID string `json:"definition_id"`
	Revision     int64  `json:"revision"`
	Enabled      bool   `json:"enabled"`
	DisplayName  string `json:"display_name"`
	RuntimeName  string `json:"runtime_name"`
}

type MCPSetupSnapshot struct {
	Version int              `json:"version"`
	Servers []MCPSetupServer `json:"servers"`
}

type MCPSetupChange struct {
	DefinitionID string          `json:"definition_id"`
	Action       string          `json:"action"`
	Before       *MCPSetupServer `json:"before,omitempty"`
	After        *MCPSetupServer `json:"after,omitempty"`
}

type MCPSetupComparison struct {
	Version               int              `json:"version"`
	Status                string           `json:"status"`
	Reason                string           `json:"reason,omitempty"`
	DesiredRevision       int64            `json:"desired_revision"`
	AppliedRevision       int64            `json:"applied_revision"`
	AttachmentAttemptID   string           `json:"attachment_attempt_id,omitempty"`
	ExcludedDefinitionIDs []string         `json:"excluded_definition_ids,omitempty"`
	SelectionIDs          []string         `json:"selection_ids,omitempty"`
	Changes               []MCPSetupChange `json:"changes,omitempty"`
}

// ResolutionInputs is an immutable catalog/selection read, never a wire payload.
type ResolutionInputs struct {
	Origins               map[string][]SelectionOrigin
	Definitions           map[string]*MCPServerDefinition
	DesiredRevision       int64
	SessionSelectionIDs   []string
	ExcludedDefinitionIDs []string
}

type ResolutionInputsRepository interface {
	ReadMCPResolutionInputs(context.Context, ResolutionContext) (*ResolutionInputs, error)
}

type AcceptedMCPSetupRepository interface {
	SaveAcceptedMCPSetup(context.Context, string, int64, string, *MCPSetupSnapshot) error
}

func ValidMCPSetup(setup *MCPSetupSnapshot) bool {
	if setup == nil || setup.Version != 1 || setup.Servers == nil || len(setup.Servers) > maxMCPSetupServers {
		return false
	}
	seen := make(map[string]bool, len(setup.Servers))
	for _, server := range setup.Servers {
		if server.DefinitionID == "" || server.Revision < 1 || server.DisplayName == "" || server.RuntimeName == "" || seen[server.DefinitionID] {
			return false
		}
		seen[server.DefinitionID] = true
	}
	encoded, err := json.Marshal(setup)
	return err == nil && len(encoded) <= maxMCPSetupBytes
}

func setupFromDefinitions(definitions map[string]*MCPServerDefinition) *MCPSetupSnapshot {
	setup := &MCPSetupSnapshot{Version: 1, Servers: make([]MCPSetupServer, 0, len(definitions))}
	for _, d := range definitions {
		if d == nil {
			return nil
		}
		setup.Servers = append(setup.Servers, MCPSetupServer{DefinitionID: d.ID, Revision: d.Revision, Enabled: d.Enabled, DisplayName: d.DisplayName, RuntimeName: d.RuntimeName})
	}
	sort.Slice(setup.Servers, func(i, j int) bool { return setup.Servers[i].DefinitionID < setup.Servers[j].DefinitionID })
	if !ValidMCPSetup(setup) {
		return nil
	}
	return setup
}

// setupFromInputs uses the same exclusion boundary for delivery and comparison.
func setupFromInputs(inputs *ResolutionInputs) *MCPSetupSnapshot {
	definitions := make(map[string]*MCPServerDefinition, len(inputs.Definitions))
	for id, definition := range inputs.Definitions {
		if !slices.Contains(inputs.ExcludedDefinitionIDs, id) {
			definitions[id] = definition
		}
	}
	return setupFromDefinitions(definitions)
}
