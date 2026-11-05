package mcpconfig

import (
	"encoding/json"
	"sort"
)

const comparisonStatusUnavailable = "unavailable"
const comparisonStatusKnown = "known"
const comparisonReasonDesiredUnavailable = "desired_unavailable"

func CompareMCPSetups(state SessionMCPSelectionState, desired *MCPSetupSnapshot) *MCPSetupComparison {
	result := &MCPSetupComparison{Version: 1, Status: comparisonStatusUnavailable, Reason: "baseline_missing", DesiredRevision: state.DesiredRevision, AppliedRevision: state.AppliedRevision, AttachmentAttemptID: state.AttachmentAttemptID}
	if !ValidMCPSetup(state.AcceptedSetup) {
		return result
	}
	if !ValidMCPSetup(desired) {
		result.Reason = comparisonReasonDesiredUnavailable
		return result
	}
	result.Status = comparisonStatusKnown
	result.Reason = ""
	result.Changes = make([]MCPSetupChange, 0)
	before := make(map[string]MCPSetupServer, len(state.AcceptedSetup.Servers))
	for _, server := range state.AcceptedSetup.Servers {
		before[server.DefinitionID] = server
	}
	for _, server := range desired.Servers {
		previous, exists := before[server.DefinitionID]
		if !exists {
			result.Changes = append(result.Changes, MCPSetupChange{DefinitionID: server.DefinitionID, Action: "add", After: &server})
		} else if previous.Revision != server.Revision || previous.Enabled != server.Enabled {
			result.Changes = append(result.Changes, MCPSetupChange{DefinitionID: server.DefinitionID, Action: "update", Before: &previous, After: &server})
		}
		delete(before, server.DefinitionID)
	}
	for _, server := range before {
		result.Changes = append(result.Changes, MCPSetupChange{DefinitionID: server.DefinitionID, Action: "remove", Before: &server})
	}
	sort.Slice(result.Changes, func(i, j int) bool { return result.Changes[i].DefinitionID < result.Changes[j].DefinitionID })
	return boundedMCPComparison(result)
}

func boundedMCPComparison(result *MCPSetupComparison) *MCPSetupComparison {
	encoded, err := json.Marshal(result)
	if len(result.Changes) > maxMCPSetupServers || len(result.ExcludedDefinitionIDs) > maxMCPSetupServers || err != nil || len(encoded) > maxMCPSetupBytes {
		result.Status = comparisonStatusUnavailable
		result.Reason = "comparison_too_large"
		result.Changes = nil
		result.SelectionIDs = nil
		result.ExcludedDefinitionIDs = nil
	}
	return result
}
