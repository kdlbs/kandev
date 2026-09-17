package lifecycle

import (
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

const assistantPolicyMetadata = "assistant_broker_policy"

func assistantRestrictedLaunch(req *LaunchRequest) bool {
	return req.McpProfile != nil && req.McpProfile.Surface == mcpprofile.SurfaceAssistantBroker
}

func validateAssistantLaunch(req *LaunchRequest) error {
	if !assistantRestrictedLaunch(req) {
		return nil
	}
	if req.ExecutorType != string(models.ExecutorTypeLocal) || req.IsPassthrough || req.RepositoryID != "" || req.RepositoryPath != "" ||
		req.SetupScript != "" || req.CopyFiles != "" || req.UseWorktree || len(req.WorkspaceFolders) != 0 {
		return fmt.Errorf("assistant policy requires a repository-free local executor without preparation scripts")
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	req.Metadata[assistantPolicyMetadata] = string(mcpprofile.SurfaceAssistantBroker)
	return nil
}

func validateAssistantCommand(req *LaunchRequest, profile *AgentProfileInfo, agent agents.Agent, version string, flags, prefix []string) error {
	if !assistantRestrictedLaunch(req) {
		return nil
	}
	if agent.ID() != "claude-acp" || version != "0.75.1" || len(flags) != 0 || len(prefix) != 0 || profile == nil ||
		profile.CLIPassthrough || len(profile.EnvVars) != 0 || len(profile.ConfigOptions) != 0 {
		return fmt.Errorf("assistant policy unsupported by the selected runtime")
	}
	return nil
}
