package lifecycle

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/scriptengine"
	"github.com/kandev/kandev/internal/task/models"
)

// dockerLaunchTarget is the daemon a container launch runs against, together
// with the identity the resulting instance reports.
//
// The local and remote Docker runtimes differ only in these values: the
// container model, bootstrap, and instance shape are the same on both.
type dockerLaunchTarget struct {
	dockerClient *docker.Client
	containerMgr *ContainerManager
	runtimeName  executor.Name
	executorType string
	logger       *logger.Logger
}

// launchDockerContainer provisions a fresh container and returns the instance
// that owns it.
func launchDockerContainer(
	ctx context.Context,
	target dockerLaunchTarget,
	req *ExecutorCreateRequest,
) (*ExecutorInstance, error) {
	baseCtx := preparationContext(ctx)

	if err := validateAgentctlStartupConfig(req.AgentctlStartupConfig); err != nil {
		return nil, fmt.Errorf("invalid agentctl configuration: %w", err)
	}
	if _, err := validateRemoteContributions(req.RemoteContributions); err != nil {
		return nil, err
	}
	if _, err := validateContributionDestinations(req.ContributionDestinations); err != nil {
		return nil, err
	}
	if _, err := validateComparisonTargets(req.ComparisonTargets); err != nil {
		return nil, err
	}

	containerCfg, err := buildDockerContainerConfig(req, target.executorType)
	if err != nil {
		return nil, fmt.Errorf("build container launch config: %w", err)
	}

	result, err := target.containerMgr.LaunchContainer(ctx, containerCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to launch container: %w", err)
	}

	containerIP, _ := target.dockerClient.GetContainerIPOn(baseCtx, result.ContainerID, containerCfg.Network.Name)
	target.logger.Info("docker instance created",
		zap.String("instance_id", req.InstanceID),
		zap.String("container_id", result.ContainerID),
		zap.String("executor_type", target.executorType))

	metadata := map[string]interface{}{MetadataKeyIsRemote: true}
	if worktreeID := getMetadataString(req.Metadata, MetadataKeyWorktreeID); worktreeID != "" {
		metadata["worktree_id"] = worktreeID
		metadata["worktree_path"] = dockerWorkspacePath
		metadata["worktree_branch"] = getMetadataString(req.Metadata, MetadataKeyWorktreeBranch)
	}

	return &ExecutorInstance{
		InstanceID:     req.InstanceID,
		TaskID:         req.TaskID,
		SessionID:      req.SessionID,
		RuntimeName:    target.runtimeName,
		Client:         result.Client,
		ContainerID:    result.ContainerID,
		ContainerIP:    containerIP,
		WorkspacePath:  dockerWorkspacePath,
		Metadata:       metadata,
		AuthToken:      result.AuthToken,
		BootstrapNonce: result.BootstrapNonce,
	}, nil
}

// buildDockerContainerConfig composes the container configuration for either
// Docker runtime. The executor type selects the default prepare script, which
// both Docker types share.
func buildDockerContainerConfig(req *ExecutorCreateRequest, executorType string) (ContainerConfig, error) {
	prepareScript, err := resolveDockerPrepareScript(req, executorType)
	if err != nil {
		return ContainerConfig{}, err
	}
	containerNet, err := resolveContainerNetwork(req.Metadata)
	if err != nil {
		return ContainerConfig{}, err
	}
	var journalHostPath, journalContainerPath string
	if req.DurableJournalHostRoot != "" && req.DurableJournalOwnerID != "" {
		location, locationErr := resolveDurableJournal(req)
		if locationErr != nil {
			return ContainerConfig{}, fmt.Errorf("resolve retained delivery journal: %w", locationErr)
		}
		journalContainerPath, err = durableJournalContainerPath(req)
		if err != nil {
			return ContainerConfig{}, err
		}
		switch executorType {
		case string(models.ExecutorTypeLocalDocker):
			capability := journal.CheckStorage(req.DurableJournalHostRoot, req.DurableJournalOwnerID)
			if !capability.Durable {
				return ContainerConfig{}, fmt.Errorf("retained delivery journal unavailable: %s", capability.Reason)
			}
			journalHostPath = location.Path
		case string(models.ExecutorTypeRemoteDocker):
			// A remote Docker daemon cannot mount a path on the backend host.
			// Its retained container owns this path until that container is removed.
		default:
			return ContainerConfig{}, fmt.Errorf("unsupported Docker executor type for durable journal: %s", executorType)
		}
	}
	return ContainerConfig{
		AgentConfig:                    req.AgentConfig,
		WorkspacePath:                  "", // Empty = no workspace mount; the clone happens inside the container.
		TaskID:                         req.TaskID,
		TaskTitle:                      req.TaskTitle,
		TaskEnvironmentID:              req.TaskEnvironmentID,
		SessionID:                      req.SessionID,
		ExecutorProfileID:              getMetadataString(req.Metadata, "executor_profile_id"),
		InstanceID:                     req.InstanceID,
		Credentials:                    req.Env,
		AutoApprovePermissions:         req.AutoApprovePermissions,
		AutoApprovePermissionsOverride: req.AutoApprovePermissionsOverride,
		McpServers:                     req.McpServers,
		McpProviders:                   req.McpProviders,
		McpProfile:                     req.McpProfile,
		PrepareScript:                  prepareScript,
		ImageTagOverride:               getMetadataString(req.Metadata, MetadataKeyImageTagOverride),
		AllowUserNamespaces:            getMetadataString(req.Metadata, MetadataKeyAllowUserNamespaces) == boolStringTrue,
		LocalClonePath:                 localCloneMountPathFor(req.Metadata, executorType),
		BaseBranches:                   getMetadataStringMap(req.Metadata, MetadataKeyBaseBranches),
		RemoteContributions:            req.RemoteContributions,
		ContributionDestinations:       req.ContributionDestinations,
		ComparisonTargets:              req.ComparisonTargets,
		AgentctlStartupConfig:          req.AgentctlStartupConfig,
		ProviderGatewayAuth:            req.ProviderGatewayAuth,
		Metadata:                       req.Metadata,
		OnProgress:                     req.OnProgress,
		Network:                        containerNet,
		DurableJournalHostPath:         journalHostPath,
		DurableJournalContainerPath:    journalContainerPath,
		DeliveryStreamID:               req.DeliveryStreamID,
		DeliveryIncarnationID:          req.DeliveryIncarnationID,
		DeliveryHarnessGeneration:      req.DeliveryHarnessGeneration,
	}, nil
}

// localCloneMountPathFor resolves the local-repository mount source, which
// only exists for a daemon sharing the backend's filesystem.
//
// A remote daemon resolves the path against its own filesystem, so forwarding
// it mounts the wrong directory or fails the launch. Remote executors require
// a cloneable origin instead; see the remote source rules.
func localCloneMountPathFor(metadata map[string]interface{}, executorType string) string {
	if executorType == string(models.ExecutorTypeRemoteDocker) {
		return ""
	}
	return localCloneMountPath(metadata)
}

// resolveDockerPrepareScript builds the in-container prepare script, falling
// back to the default for the executor type when the profile has none.
func resolveDockerPrepareScript(req *ExecutorCreateRequest, executorType string) (string, error) {
	script := getMetadataString(req.Metadata, MetadataKeySetupScript)
	if script == "" {
		script = DefaultPrepareScript(executorType)
	}
	if script == "" {
		return "", nil
	}
	options, err := primaryCheckoutOptions(req.Metadata)
	if err != nil {
		return "", err
	}
	script, err = checkoutOptionsPrepareScript(script, options)
	if err != nil {
		return "", err
	}
	// A sparse checkout has to finish before the repository setup script runs,
	// so the placeholder moves to the tail below rather than staying inline.
	deferSetup := options != nil && len(options.SparseDirectories) > 0
	if deferSetup {
		script = strings.Replace(script, "{{repository.setup_script}}", "", 1)
	}
	script = withBranchCheckout(req, script)
	if binding, ok := req.RemoteContributions[""]; ok {
		contributionScript, err := scriptengine.RemoteContributionSetupScript(&binding)
		if err != nil {
			return "", err
		}
		script += contributionScript
	}
	script += checkoutOptionsValidationScript(options)
	if destination, ok := req.ContributionDestinations[""]; ok {
		destinationScript, err := scriptengine.ContributionDestinationSetupScript(&destination)
		if err != nil {
			return "", err
		}
		script += destinationScript
	}

	if deferSetup {
		script += "\n" + selectedCheckoutCredentialScrubScript(req.Metadata) + "\n{{repository.setup_script}}\n"
	}

	// Placeholder resolution uses in-container paths, which are the same on
	// both Docker runtimes: the workspace is /workspace and the image has the
	// agent and agentctl already present, so install/start resolve to no-ops.
	resolver := scriptengine.NewResolver().
		WithProvider(scriptengine.WorkspaceProvider(dockerWorkspacePath)).
		WithProvider(scriptengine.GitIdentityProvider(req.Metadata)).
		WithProvider(scriptengine.GitHubAuthProvider(req.Env)).
		WithProvider(scriptengine.WorktreeProvider(
			"",
			dockerWorkspacePath,
			getMetadataString(req.Metadata, MetadataKeyWorktreeID),
			getMetadataString(req.Metadata, MetadataKeyWorktreeBranch),
			getMetadataString(req.Metadata, MetadataKeyBaseBranch),
		)).
		WithProvider(scriptengine.RepositoryProvider(
			req.Metadata,
			req.Env,
			getGitRemoteURL,
			injectGitHubTokenIntoCloneURL,
		)).
		WithProvider(scriptengine.AgentInstallProvider(nil)).
		WithStatic(map[string]string{
			"kandev.agentctl.port":    "9999",
			"kandev.agentctl.install": "",
			"kandev.agentctl.start":   "",
		})

	return resolver.Resolve(script), nil
}

// stopDockerContainer applies the Docker stop policy to one container.
//
// An ordinary stop preserves the container: it holds the cloned workspace and
// the agentctl process that a resume re-attaches to. Only a forced stop or a
// destructive task/session lifecycle reason removes it.
func stopDockerContainer(
	ctx context.Context,
	dockerClient *docker.Client,
	containerMgr *ContainerManager,
	instance *ExecutorInstance,
	force bool,
	log *logger.Logger,
) error {
	cleanupCtx, cancel := dockerCleanupContext(ctx, instance.AgentStopFailed)
	defer cancel()

	if force {
		if killErr := dockerClient.KillContainer(cleanupCtx, instance.ContainerID, "SIGKILL"); killErr != nil {
			log.Warn("failed to kill docker container before forced removal",
				zap.String("container_id", instance.ContainerID),
				zap.Error(killErr))
		}
		if removeErr := dockerClient.RemoveContainer(cleanupCtx, instance.ContainerID, true); removeErr != nil {
			return fmt.Errorf("failed to remove container after kill: %w", removeErr)
		}
		return nil
	}

	if shouldRunExecutorCleanup(instance.StopReason) {
		if err := containerMgr.StopContainer(cleanupCtx, instance.ContainerID, dockerStopContainerTimeout); err != nil {
			return fmt.Errorf("failed to stop and remove container: %w", err)
		}
		return nil
	}

	if err := dockerClient.StopContainer(cleanupCtx, instance.ContainerID, dockerStopContainerTimeout); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	return nil
}
