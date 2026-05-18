// Package onboarding provides onboarding state, completion, and FS import logic.
package onboarding

// OnboardingFSWorkspace represents a workspace found on the filesystem.
type OnboardingFSWorkspace struct {
	Name string `json:"name"`
}

// OnboardingStateResponse is the response for GET /onboarding-state.
type OnboardingStateResponse struct {
	Completed    bool                    `json:"completed"`
	WorkspaceID  string                  `json:"workspaceId,omitempty"`
	CEOAgentID   string                  `json:"ceoAgentId,omitempty"`
	FSWorkspaces []OnboardingFSWorkspace `json:"fsWorkspaces"`
}

// OnboardingImportFSResponse is the response for POST /onboarding/import-fs.
type OnboardingImportFSResponse struct {
	WorkspaceIDs  []string `json:"workspaceIds"`
	ImportedCount int      `json:"importedCount"`
}

// OnboardingCompleteRequest is the request body for POST /onboarding/complete.
type OnboardingCompleteRequest struct {
	WorkspaceName      string `json:"workspaceName"`
	TaskPrefix         string `json:"taskPrefix"`
	AgentName          string `json:"agentName"`
	AgentProfileID     string `json:"agentProfileId"`
	ExecutorPreference string `json:"executorPreference"`
	TaskTitle          string `json:"taskTitle,omitempty"`
	TaskDescription    string `json:"taskDescription,omitempty"`
	// DefaultTier is the workspace routing default tier captured in the
	// onboarding wizard. Valid values: "frontier", "balanced", "economy".
	// Empty or invalid values default to "balanced" (silent — onboarding
	// should never fail because of an unknown tier label).
	DefaultTier string `json:"default_tier,omitempty"`
}

// OnboardingCompleteResponse is the response for POST /onboarding/complete.
type OnboardingCompleteResponse struct {
	WorkspaceID string `json:"workspaceId"`
	AgentID     string `json:"agentId"`
	TaskID      string `json:"taskId,omitempty"`
}
