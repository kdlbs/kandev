package mcpconfig

import "errors"

// AutomationMCPContribution is captured by the trusted launch builder and retained with that process.
type AutomationMCPContribution struct {
	AutomationID  string
	WorkspaceID   string
	TaskID        string
	Revision      int64
	DefinitionIDs []string
}

// AddAutomationMCPOrigins adds references before catalog rows are read in the same snapshot.
func AddAutomationMCPOrigins(rc ResolutionContext, origins map[string][]SelectionOrigin) error {
	a := rc.Automation
	if a == nil {
		return nil
	}
	if a.AutomationID == "" || a.WorkspaceID != rc.WorkspaceID || a.TaskID != rc.TaskID || a.TaskID == "" || a.Revision < 1 {
		return errors.New("automation MCP contribution owner mismatch")
	}
	addOrigins(origins, a.DefinitionIDs, SelectionOrigin{Scope: SelectionScope("automation"), WorkspaceID: rc.WorkspaceID, OwnerID: a.AutomationID})
	return nil
}
