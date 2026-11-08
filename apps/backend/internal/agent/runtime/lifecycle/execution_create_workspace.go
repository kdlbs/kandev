package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/common/mcpmode"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

type executionCreateWorkspaceInputs struct {
	taskID             string
	executionID        string
	profileInfo        *AgentProfileInfo
	managedGoCachePath string
}

// prepareExecutionWorkspace resolves retained workspace bindings, permissions
// and control authentication before request assembly reveals the bootstrap nonce.
func (m *Manager) prepareExecutionWorkspace(ctx context.Context, info *WorkspaceInfo, inputs executionCreateWorkspaceInputs) (*ExecutorCreateRequest, error) {
	metadata := copyExecutionCreateMetadata(info.Metadata)
	captureResourceCircuitIdentity(metadata, inputs.executionID, inputs.profileInfo)
	delete(metadata, mcpprofile.BuiltinBindingMetadataKey)
	m.seedExecutionBaseBranches(ctx, inputs.taskID, inputs.executionID, metadata)
	if inputs.managedGoCachePath != "" {
		metadata[managedGoCacheMetadataKey] = inputs.managedGoCachePath
	}
	remoteContributions, err := remoteContributionsFromMetadata(metadata)
	if err != nil {
		return nil, err
	}
	contributionDestinations, err := contributionDestinationsFromMetadata(metadata)
	if err != nil {
		return nil, err
	}
	comparisonTargets, err := comparisonTargetsFromMetadata(metadata)
	if err != nil {
		return nil, err
	}
	if len(comparisonTargets) == 0 {
		comparisonTargets, err = comparisonTargetsFromWorkspaceRepositories(info.WorkspaceRepositories, info.ExecutorType)
		if err != nil {
			return nil, err
		}
	}
	autoApprove := false
	var autoApproveOverride *bool
	if inputs.profileInfo != nil {
		autoApprove = inputs.profileInfo.AutoApprove
		autoApproveOverride = boolPtr(inputs.profileInfo.AutoApprove)
	}
	// Coordinator permission policy permits only its exact tool set, independent
	// of the historical profile's general auto-approve preference.
	if info.McpMode == mcpmode.Coordinator {
		autoApprove = false
		autoApproveOverride = boolPtr(false)
	}
	authToken := m.revealRuntimeSecret(ctx, info.Metadata, MetadataKeyAuthTokenSecret)
	if isDockerExecutorType(info.ExecutorType) {
		controlToken, err := m.revealContainerControlAuthToken(ctx, info.Metadata, getMetadataString(info.Metadata, MetadataKeyContainerID) != "")
		if err != nil {
			return nil, fmt.Errorf("resolve container control token: %w", err)
		}
		if controlToken != "" {
			authToken = controlToken
		}
	}
	return &ExecutorCreateRequest{
		Metadata: metadata, RemoteContributions: remoteContributions,
		ContributionDestinations: contributionDestinations, ComparisonTargets: comparisonTargets,
		AutoApprovePermissions: autoApprove, AutoApprovePermissionsOverride: autoApproveOverride,
		AuthToken: authToken,
	}, nil
}

// A recreated execution receives the retained metadata except for the managed
// cache path, which is resolved for this admission rather than copied forward.
func copyExecutionCreateMetadata(source map[string]interface{}) map[string]interface{} {
	metadata := make(map[string]interface{}, len(source)+1)
	for key, value := range source {
		metadata[key] = value
	}
	delete(metadata, managedGoCacheMetadataKey)
	return metadata
}
