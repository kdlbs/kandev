package lifecycle

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
)

func validTestRemoteContribution(number int, sourcePath string) models.RemoteContribution {
	return models.RemoteContribution{
		Version:      models.RemoteContributionVersion,
		Provider:     models.RemoteContributionProviderGitHub,
		Kind:         models.RemoteContributionKindPullRequest,
		CanonicalURL: "https://github.com/acme/widget/pull/" + strconv.Itoa(number),
		Number:       number,
		State:        models.RemoteContributionStateOpen,
		BaseBranch:   "main",
		HeadBranch:   "feature/remote",
		HeadSHA:      strings.Repeat("a", 40),
		SourceRepository: models.RemoteContributionRepository{
			Host: "github.com", Path: sourcePath, RemoteURL: "https://github.com/" + sourcePath + ".git",
		},
		CollaborationAllowed: true,
	}
}

// TestBuildLaunchMetadataExecutorConfigWinsForTrustedKeys pins the security
// boundary: connection-routing keys always come from the configured executor
// record, while ordinary per-task keys keep the caller's value.
func TestBuildLaunchMetadataExecutorConfigWinsForTrustedKeys(t *testing.T) {
	req := &LaunchRequest{
		Metadata: map[string]interface{}{
			MetadataKeySSHHost:            "attacker.example.com",
			MetadataKeySSHHostFingerprint: "SHA256:forged",
			MetadataKeySSHUser:            "root",
			MetadataKeyCleanupScript:      "task-cleanup.sh",
		},
		ExecutorConfig: map[string]string{
			MetadataKeySSHHost:            "trusted.example.com",
			MetadataKeySSHHostFingerprint: "SHA256:pinned",
			MetadataKeySSHUser:            "deploy",
			MetadataKeyCleanupScript:      "executor-cleanup.sh",
			MetadataKeySSHWorkdirRoot:     "/srv/kandev",
		},
	}

	metadata := buildLaunchMetadata(req, "", "", "")

	require.Equal(t, "trusted.example.com", metadata[MetadataKeySSHHost],
		"a task metadata payload must never be able to pivot the SSH host")
	require.Equal(t, "SHA256:pinned", metadata[MetadataKeySSHHostFingerprint],
		"the pinned host key must not be overridable by request metadata")
	require.Equal(t, "deploy", metadata[MetadataKeySSHUser])
	require.Equal(t, "task-cleanup.sh", metadata[MetadataKeyCleanupScript],
		"untrusted keys keep the caller's value when present")
	require.Equal(t, "/srv/kandev", metadata[MetadataKeySSHWorkdirRoot],
		"untrusted executor-config keys still fill in when the caller supplied nothing")
}

func TestBuildLaunchMetadataExecutorConfigWinsForKubernetesConnectionKeys(t *testing.T) {
	req := &LaunchRequest{
		ExecutorType: string(models.ExecutorTypeKubernetes),
		Metadata: map[string]interface{}{
			"auth_mode":               "in_cluster",
			"kubeconfig_path":         "/tmp/attacker-kubeconfig",
			"kube_context":            "attacker",
			"namespace":               "attacker",
			"request_timeout_seconds": "300",
		},
		ExecutorConfig: map[string]string{
			"auth_mode":               "kubeconfig",
			"kubeconfig_path":         "/etc/kandev/cluster.yaml",
			"namespace":               "kandev-agents",
			"request_timeout_seconds": "30",
		},
	}

	metadata := buildLaunchMetadata(req, "", "", "")

	require.Equal(t, "kubeconfig", metadata["auth_mode"])
	require.Equal(t, "/etc/kandev/cluster.yaml", metadata["kubeconfig_path"])
	require.Empty(t, metadata["kube_context"],
		"an omitted trusted context must clear task metadata instead of selecting another context")
	require.Equal(t, "kandev-agents", metadata["namespace"])
	require.Equal(t, "30", metadata["request_timeout_seconds"])
}

func TestLaunchBuildExecutorRequestUsesAuthoritativeKubernetesProfileConfig(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &createInstanceExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}
	registry.Register(backend)
	reader := &fakeExecutorProfileReader{profiles: map[string]*models.ExecutorProfile{
		"profile-1": {
			ID:         "profile-1",
			ExecutorID: "executor-1",
			Config: map[string]string{
				MetadataKeyKubernetesProfilePlatform:      "linux/arm64",
				MetadataKeyKubernetesProfileMainContainer: "trusted-main",
				MetadataKeyKubernetesPodTemplateYAML:      "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n      - name: trusted-main\n        image: example.test/agent:latest\n",
				MetadataKeyKubernetesWorkspaceMode:        "existing_claim",
				MetadataKeyKubernetesWorkspaceClaimName:   "trusted-claim",
			},
		},
	}}
	mgr := &Manager{
		executorRegistry:       registry,
		executorFallbackPolicy: ExecutorFallbackDeny,
		executorProfileReader:  reader,
		logger:                 log,
	}
	req := &LaunchRequest{
		ExecutorType:         string(models.ExecutorTypeKubernetes),
		EnvironmentFinalized: true,
		Metadata: map[string]interface{}{
			"executor_id":                              "executor-1",
			MetadataKeyExecutorProfileID:               "profile-1",
			MetadataKeyKubernetesProfilePlatform:       "linux/amd64",
			MetadataKeyKubernetesProfileMainContainer:  "attacker-main",
			MetadataKeyKubernetesPodTemplateYAML:       "attacker-template",
			MetadataKeyKubernetesWorkspaceMode:         "managed_pvc",
			MetadataKeyKubernetesWorkspaceSize:         "1Ti",
			MetadataKeyKubernetesWorkspaceStorageClass: "attacker-class",
			MetadataKeyKubernetesWorkspaceAccessModes:  "ReadWriteOnce",
			MetadataKeyKubernetesWorkspaceClaimName:    "attacker-claim",
		},
	}

	_, _, _, err := mgr.launchBuildExecutorRequest(
		context.Background(), "instance-1", req, &testAgent{id: "agent"}, &AgentProfileInfo{}, "", "", "", nil,
	)

	require.NoError(t, err)
	require.Equal(t, []string{"profile-1"}, reader.profileArgs)
	require.NotNil(t, backend.lastRequest)
	metadata := backend.lastRequest.Metadata
	require.Equal(t, "linux/arm64", metadata[MetadataKeyKubernetesProfilePlatform])
	require.Equal(t, "trusted-main", metadata[MetadataKeyKubernetesProfileMainContainer])
	require.Contains(t, metadata[MetadataKeyKubernetesPodTemplateYAML], "name: trusted-main")
	require.Equal(t, "existing_claim", metadata[MetadataKeyKubernetesWorkspaceMode])
	require.Equal(t, "trusted-claim", metadata[MetadataKeyKubernetesWorkspaceClaimName])
	require.Empty(t, metadata[MetadataKeyKubernetesWorkspaceAccessModes])
	require.Empty(t, metadata[MetadataKeyKubernetesWorkspaceStorageClass])
	require.Empty(t, getMetadataString(metadata, MetadataKeyKubernetesWorkspaceSize),
		"an omitted profile field must erase a task-supplied value")
}

func TestLaunchBuildExecutorRequestCanonicalizesLegacyKubernetesWorkspaceKeys(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &createInstanceExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}
	registry.Register(backend)
	reader := &fakeExecutorProfileReader{profiles: map[string]*models.ExecutorProfile{
		"profile-legacy": {
			ID: "profile-legacy", ExecutorID: "executor-1",
			Config: map[string]string{
				"platform":                "linux/amd64",
				"main_container":          "kandev-agent",
				"pod_template_yaml":       validKubernetesProfilePodTemplate,
				"workspace_mode":          "managed_pvc",
				"workspace_size":          "8Gi",
				"workspace_storage_class": "fast",
				"workspace_access_modes":  `["ReadWriteOnce","ReadOnlyMany"]`,
				"workspace.claim_name":    "",
			},
		},
	}}
	mgr := &Manager{
		executorRegistry: registry, executorFallbackPolicy: ExecutorFallbackDeny,
		executorProfileReader: reader, logger: log,
	}
	req := &LaunchRequest{
		ExecutorType: string(models.ExecutorTypeKubernetes), EnvironmentFinalized: true,
		Metadata: map[string]interface{}{
			"executor_id": "executor-1", MetadataKeyExecutorProfileID: "profile-legacy",
			MetadataKeyKubernetesWorkspaceMode: "empty_dir",
		},
	}

	_, _, _, err := mgr.launchBuildExecutorRequest(
		context.Background(), "instance-legacy", req, &testAgent{id: "agent"}, &AgentProfileInfo{}, "", "", "", nil,
	)

	require.NoError(t, err)
	require.NotNil(t, backend.lastRequest)
	metadata := backend.lastRequest.Metadata
	require.Equal(t, "managed_pvc", metadata[MetadataKeyKubernetesWorkspaceMode])
	require.Equal(t, "8Gi", metadata[MetadataKeyKubernetesWorkspaceSize])
	require.Equal(t, "fast", metadata[MetadataKeyKubernetesWorkspaceStorageClass])
	require.Equal(t, `["ReadWriteOnce","ReadOnlyMany"]`, metadata[MetadataKeyKubernetesWorkspaceAccessModes])
}

const validKubernetesProfilePodTemplate = "apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n      - name: kandev-agent\n        image: example.test/agent:latest\n"

func TestLaunchBuildExecutorRequestUsesRecordedKubernetesSnapshotWhenProfileWasDeleted(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &createInstanceExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}
	registry.Register(backend)
	reader := &fakeExecutorProfileReader{profiles: map[string]*models.ExecutorProfile{}}
	mgr := &Manager{
		executorRegistry:       registry,
		executorFallbackPolicy: ExecutorFallbackDeny,
		executorProfileReader:  reader,
		logger:                 log,
	}
	snapshot := `{"platform":"linux/amd64","main_container":"kandev-agent","pod_template_yaml":"apiVersion: v1\nkind: PodTemplate\ntemplate:\n  spec:\n    containers:\n      - name: kandev-agent\n        image: example.test/agent:latest\n","workspace":{"mode":"managed_pvc","size":"1Gi","access_modes":["ReadWriteOnce"]}}`
	req := &LaunchRequest{
		ExecutorType:         string(models.ExecutorTypeKubernetes),
		EnvironmentFinalized: true,
		PreviousExecutionID:  "previous-instance",
		Metadata:             completeKubernetesRecordedResumeMetadata(snapshot),
	}
	req.Metadata[MetadataKeyExecutorProfileID] = "deleted-profile"
	req.Metadata[MetadataKeyKubernetesProfilePlatform] = "linux/arm64"
	req.Metadata[MetadataKeyKubernetesPodTemplateYAML] = "attacker-template"
	req.Metadata[MetadataKeyKubernetesWorkspaceMode] = "empty_dir"

	_, _, _, err := mgr.launchBuildExecutorRequest(
		context.Background(), "instance-2", req, &testAgent{id: "agent"}, &AgentProfileInfo{}, "", "", "", nil,
	)

	require.NoError(t, err)
	require.Empty(t, reader.profileArgs, "retained workloads must not depend on the current profile row")
	require.NotNil(t, backend.lastRequest)
	require.Equal(t, snapshot, backend.lastRequest.Metadata[MetadataKeyKubernetesProfileSnapshot])
}

func TestLaunchBuildExecutorRequestDoesNotBypassProfileForIncompleteKubernetesInventory(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&createInstanceExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}})
	reader := &fakeExecutorProfileReader{profiles: map[string]*models.ExecutorProfile{}}
	mgr := &Manager{
		executorRegistry:       registry,
		executorFallbackPolicy: ExecutorFallbackDeny,
		executorProfileReader:  reader,
		logger:                 log,
	}
	req := &LaunchRequest{
		ExecutorType:         string(models.ExecutorTypeKubernetes),
		EnvironmentFinalized: true,
		PreviousExecutionID:  "previous-instance",
		Metadata: map[string]interface{}{
			"executor_id":                        "executor-1",
			MetadataKeyExecutorProfileID:         "missing-profile",
			MetadataKeyKubernetesPodName:         "kandev-recorded",
			MetadataKeyKubernetesProfileSnapshot: `{}`,
		},
	}

	_, _, _, err := mgr.launchBuildExecutorRequest(
		context.Background(), "instance-2", req, &testAgent{id: "agent"}, &AgentProfileInfo{}, "", "", "", nil,
	)

	require.Error(t, err)
	require.Equal(t, []string{"missing-profile"}, reader.profileArgs)
}

func completeKubernetesRecordedResumeMetadata(snapshot string) map[string]interface{} {
	return map[string]interface{}{
		"executor_id":                              "executor-1",
		MetadataKeyExecutorProfileID:               "profile-1",
		MetadataKeyKubernetesNamespace:             "kandev-agents",
		MetadataKeyKubernetesPodName:               "kandev-recorded",
		MetadataKeyKubernetesPodUID:                "pod-uid",
		MetadataKeyKubernetesMainContainer:         "kandev-agent",
		MetadataKeyKubernetesRuntimeWorkspaceMode:  "empty_dir",
		MetadataKeyKubernetesAgentctlRemotePort:    "41001",
		MetadataKeyKubernetesAgentctlInstanceID:    "previous-instance",
		MetadataKeyKubernetesResourceExecutorID:    "executor-1",
		MetadataKeyKubernetesResourceProfileID:     "profile-1",
		MetadataKeyKubernetesResourceInstanceID:    "previous-instance",
		MetadataKeyKubernetesResourceTaskID:        "task-1",
		MetadataKeyKubernetesResourceSessionID:     "session-1",
		MetadataKeyKubernetesResourceEnvironmentID: "environment-1",
		MetadataKeyKubernetesExecutorConfigHash:    "executor-hash",
		MetadataKeyKubernetesProfileConfigHash:     "profile-hash",
		MetadataKeyKubernetesTemplateHash:          "template-hash",
		MetadataKeyKubernetesProfileSnapshot:       snapshot,
	}
}

func TestIsTrustedExecutorConfigKey(t *testing.T) {
	for _, key := range []string{
		MetadataKeySSHHost, MetadataKeySSHHostAlias, MetadataKeySSHPort, MetadataKeySSHUser,
		MetadataKeySSHHostFingerprint, MetadataKeySSHIdentitySource, MetadataKeySSHIdentityFile,
		MetadataKeySSHProxyJump,
	} {
		require.True(t, isTrustedExecutorConfigKey(key), "%s steers the connection and must be trusted-only", key)
	}
	for _, key := range []string{
		MetadataKeySSHWorkdirRoot, MetadataKeySSHShell, MetadataKeyCleanupScript, MetadataKeySetupScript,
	} {
		require.False(t, isTrustedExecutorConfigKey(key), "%s is per-task config, not connection routing", key)
	}
}

func TestBuildLaunchMetadataProjectsWorktreeAndRepoFields(t *testing.T) {
	req := &LaunchRequest{
		RepositoryPath: "/repos/widget",
		SetupScript:    "make setup",
		BaseBranch:     "main",
	}

	metadata := buildLaunchMetadata(req, "/repos/widget/.git", "wt-1", "kandev/feature")

	require.Equal(t, "/repos/widget/.git", metadata[MetadataKeyMainRepoGitDir])
	require.Equal(t, "wt-1", metadata[MetadataKeyWorktreeID])
	require.Equal(t, "kandev/feature", metadata[MetadataKeyWorktreeBranch])
	require.Equal(t, "/repos/widget", metadata[MetadataKeyRepositoryPath])
	require.Equal(t, "make setup", metadata[MetadataKeySetupScript])
	require.Equal(t, "main", metadata[MetadataKeyBaseBranch])
	require.Equal(t, map[string]string{"": "main"}, metadata[MetadataKeyBaseBranches],
		"the single-repo base branch is recorded under the empty key for single-repo trackers")
}

func TestBuildLaunchMetadataOmitsEmptyOptionalKeys(t *testing.T) {
	metadata := buildLaunchMetadata(&LaunchRequest{}, "", "", "")

	for _, key := range []string{
		MetadataKeyMainRepoGitDir, MetadataKeyWorktreeID, MetadataKeyWorktreeBranch,
		MetadataKeyRepositoryPath, MetadataKeySetupScript, MetadataKeyBaseBranch, MetadataKeyBaseBranches,
	} {
		require.NotContains(t, metadata, key, "%s must be absent when the request carries no value", key)
	}
}

func TestCollectRemoteContributionsSingleRepoUsesRootKey(t *testing.T) {
	binding := validTestRemoteContribution(7, "contributor/widget")
	req := &LaunchRequest{RemoteContribution: &binding}

	bindings, err := collectRemoteContributions(req)

	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Contains(t, bindings, "", "a repo-less launch binds the workspace root")
	require.Equal(t, binding.CanonicalURL, bindings[""].CanonicalURL)
}

func TestCollectRemoteContributionsMultiRepoKeysSiblings(t *testing.T) {
	first := validTestRemoteContribution(7, "contributor/widget")
	second := validTestRemoteContribution(9, "contributor/gadget")
	req := &LaunchRequest{
		Repositories: []RepoLaunchSpec{
			{RepositoryID: "r1", RepoName: "widget", RemoteContribution: &first},
			{RepositoryID: "r2", RepoName: "gadget", BranchSlug: "hotfix", RemoteContribution: &second},
		},
	}

	bindings, err := collectRemoteContributions(req)

	require.NoError(t, err)
	require.Len(t, bindings, 2)
	require.Equal(t, first.CanonicalURL, bindings[""].CanonicalURL,
		"the first repository owns the workspace root")
	require.Equal(t, second.CanonicalURL, bindings["gadget-hotfix"].CanonicalURL,
		"siblings use the same deterministic key as base-branch projection")
}

func TestCollectRemoteContributionsRejectsConflictingBindings(t *testing.T) {
	first := validTestRemoteContribution(7, "contributor/widget")
	second := validTestRemoteContribution(9, "contributor/widget")
	req := &LaunchRequest{
		Repositories: []RepoLaunchSpec{
			{RepositoryID: "r1", RepoName: "widget", RemoteContribution: &first},
			// Same key (index > 0 but identical name/slug) with a different URL.
			{RepositoryID: "r2", RepoName: "widget", RemoteContribution: &second},
			{RepositoryID: "r3", RepoName: "widget", RemoteContribution: &second},
		},
	}

	_, err := collectRemoteContributions(req)
	require.NoError(t, err, "identical sibling URLs are not a conflict")

	third := validTestRemoteContribution(11, "contributor/widget")
	req.Repositories[2].RemoteContribution = &third
	_, err = collectRemoteContributions(req)
	require.ErrorContains(t, err, "multiple remote contributions target workspace repository")
}

func TestCollectRemoteContributionsPropagatesValidationFailure(t *testing.T) {
	invalid := validTestRemoteContribution(7, "contributor/widget")
	invalid.State = "closed"

	t.Run("single repo", func(t *testing.T) {
		_, err := collectRemoteContributions(&LaunchRequest{RemoteContribution: &invalid})
		require.ErrorContains(t, err, "validate remote contribution")
	})

	t.Run("multi repo names the repository", func(t *testing.T) {
		_, err := collectRemoteContributions(&LaunchRequest{
			Repositories: []RepoLaunchSpec{{RepositoryID: "r1", RepoName: "widget", RemoteContribution: &invalid}},
		})
		require.ErrorContains(t, err, `validate remote contribution for repository "widget"`)
	})
}

func TestCollectRemoteContributionsEmptyInputs(t *testing.T) {
	bindings, err := collectRemoteContributions(nil)
	require.NoError(t, err)
	require.Nil(t, bindings)

	bindings, err = collectRemoteContributions(&LaunchRequest{})
	require.NoError(t, err)
	require.Nil(t, bindings)

	bindings, err = collectRemoteContributions(&LaunchRequest{
		Repositories: []RepoLaunchSpec{{RepositoryID: "r1", RepoName: "widget"}},
	})
	require.NoError(t, err)
	require.Nil(t, bindings, "repositories without bindings produce no map at all")
}

// TestBuildEnvPrepareRequestForwardsMultiRepoSpecs pins the per-repo
// RepoSetupScript inheritance: an entry without its own script inherits the
// request-level one, and an entry with one keeps it.
func TestBuildEnvPrepareRequestForwardsMultiRepoSpecs(t *testing.T) {
	req := &LaunchRequest{
		TaskID:      "task-1",
		WorkspaceID: "ws-1",
		SessionID:   "session-1",
		TaskTitle:   "Fix the widget",
		Metadata: map[string]interface{}{
			MetadataKeyRepoSetupScript: "shared-setup.sh",
			MetadataKeyWorktreeBranch:  "kandev/fix-widget",
		},
		Repositories: []RepoLaunchSpec{
			{RepositoryID: "r1", RepoName: "widget", BaseBranch: "main"},
			{RepositoryID: "r2", RepoName: "gadget", RepoSetupScript: "gadget-setup.sh", PRNumber: 12},
		},
	}

	prep := buildEnvPrepareRequest(req, "/work/task-1", executor.NameStandalone)

	require.Equal(t, "task-1", prep.TaskID)
	require.Equal(t, "ws-1", prep.WorkspaceID)
	require.Equal(t, "Fix the widget", prep.TaskTitle)
	require.Equal(t, executor.NameStandalone, prep.ExecutorType)
	require.Equal(t, "/work/task-1", prep.WorkspacePath)
	require.Equal(t, "shared-setup.sh", prep.RepoSetupScript)
	require.Equal(t, "kandev/fix-widget", prep.WorktreeBranch,
		"the worktree branch is read out of launch metadata, not a request field")

	require.Len(t, prep.Repositories, 2)
	require.Equal(t, "shared-setup.sh", prep.Repositories[0].RepoSetupScript,
		"a repo without its own setup script inherits the request-level one")
	require.Equal(t, "main", prep.Repositories[0].BaseBranch)
	require.Equal(t, "gadget-setup.sh", prep.Repositories[1].RepoSetupScript,
		"a repo with its own setup script keeps it")
	require.Equal(t, 12, prep.Repositories[1].PRNumber)
}

func TestBuildEnvPrepareRequestSingleRepoLeavesRepositoriesEmpty(t *testing.T) {
	req := &LaunchRequest{
		TaskID:         "task-1",
		RepositoryPath: "/repos/widget",
		UseWorktree:    true,
		BaseBranch:     "main",
		Env:            map[string]string{"FOO": "bar"},
	}

	prep := buildEnvPrepareRequest(req, "/work/task-1", executor.NameDocker)

	require.Empty(t, prep.Repositories, "a legacy single-repo launch carries no per-repo spec list")
	require.Equal(t, "/repos/widget", prep.RepositoryPath)
	require.True(t, prep.UseWorktree)
	require.Equal(t, map[string]string{"FOO": "bar"}, prep.Env)
	require.Empty(t, prep.RepoSetupScript)
}

func TestExecutionProfileIDPrefersExplicitExecutionProfile(t *testing.T) {
	require.Empty(t, executionProfileID(nil))
	require.Equal(t, "agent-profile", executionProfileID(&LaunchRequest{AgentProfileID: "agent-profile"}))
	require.Equal(t, "exec-profile", executionProfileID(&LaunchRequest{
		AgentProfileID: "agent-profile", ExecutionProfileID: "exec-profile",
	}), "a concrete execution profile wins over the Office identity")
}

func TestApplyRouteOverrideToProfileReplacesOnlyExplicitValues(t *testing.T) {
	profile := &AgentProfileInfo{AgentName: "claude-acp", Model: "sonnet", Mode: "default"}

	applyRouteOverrideToProfile(profile, &LaunchRequest{RouteOverride: &RouteOverride{Model: "opus"}})

	require.Equal(t, "claude-acp", profile.AgentName, "an empty ProviderID must not clear the profile agent")
	require.Equal(t, "opus", profile.Model)
	require.Empty(t, profile.Mode, "routing owns the mode, so it is applied unconditionally")
}

func TestApplyRouteOverrideToProfileNilInputs(t *testing.T) {
	profile := &AgentProfileInfo{AgentName: "claude-acp", Mode: "plan"}

	applyRouteOverrideToProfile(nil, &LaunchRequest{RouteOverride: &RouteOverride{ProviderID: "codex"}})
	applyRouteOverrideToProfile(profile, nil)
	applyRouteOverrideToProfile(profile, &LaunchRequest{})

	require.Equal(t, "claude-acp", profile.AgentName)
	require.Equal(t, "plan", profile.Mode, "no override means no mutation at all")
}

func TestRuntimeEnvFromMetadataAcceptsBothShapes(t *testing.T) {
	require.Empty(t, runtimeEnvFromMetadata(nil))
	require.Empty(t, runtimeEnvFromMetadata(map[string]interface{}{}))

	typed := runtimeEnvFromMetadata(map[string]interface{}{
		"runtime_env": map[string]string{"ANTHROPIC_API_KEY": "sk-live", "PATH": "/usr/bin"},
	})
	require.Equal(t, map[string]string{"ANTHROPIC_API_KEY": "sk-live", "PATH": "/usr/bin"}, typed,
		"the in-memory shape must reach the agent subprocess verbatim")

	decoded := runtimeEnvFromMetadata(map[string]interface{}{
		"runtime_env": map[string]interface{}{"ANTHROPIC_API_KEY": "sk-live", "PORT": 8080},
	})
	require.Equal(t, map[string]string{"ANTHROPIC_API_KEY": "sk-live"}, decoded,
		"a JSON round-tripped map keeps string values and drops non-strings")
}

func TestGetAttachmentsFromMetadata(t *testing.T) {
	exec := &AgentExecution{ID: "exec-1"}
	require.Nil(t, getAttachmentsFromMetadata(exec))

	exec.setMetadataValue("attachments", "not-a-slice")
	require.Nil(t, getAttachmentsFromMetadata(exec), "a wrong-typed value must not panic or leak through")

	want := []MessageAttachment{{AttachmentID: "att-1", Type: "image", MimeType: "image/png"}}
	exec.setMetadataValue("attachments", want)
	require.Equal(t, want, getAttachmentsFromMetadata(exec))
}

// TestSetExecutionEnvStoresDefensiveCopy pins the Runtime().Env contract at the
// execution boundary: what SetExecutionEnv stores is exactly what
// configureAndStartAgent later hands to the agentctl child process, and the
// caller's map cannot mutate it after the fact.
func TestSetExecutionEnvStoresDefensiveCopy(t *testing.T) {
	mgr := newTestManager(t)
	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	require.NoError(t, mgr.executionStore.Add(exec))

	env := map[string]string{"ANTHROPIC_BASE_URL": "https://proxy.internal", "KANDEV_TASK_ID": "task-1"}
	require.NoError(t, mgr.SetExecutionEnv(context.Background(), "exec-1", env))
	env["ANTHROPIC_BASE_URL"] = "https://attacker.example"

	require.Equal(t,
		map[string]string{"ANTHROPIC_BASE_URL": "https://proxy.internal", "KANDEV_TASK_ID": "task-1"},
		runtimeEnvFromMetadata(exec.MetadataSnapshot()),
		"the stored runtime env must be a copy taken at call time")
}

func TestSetExecutionEnvAndDescriptionRejectUnknownExecution(t *testing.T) {
	mgr := newTestManager(t)

	require.ErrorContains(t, mgr.SetExecutionEnv(context.Background(), "missing", nil), `execution "missing" not found`)
	require.ErrorContains(t, mgr.SetExecutionDescription(context.Background(), "missing", "x"),
		`execution "missing" not found`)
}

func TestSetExecutionDescriptionFeedsPassthroughPromptInjection(t *testing.T) {
	mgr := newTestManager(t)
	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	require.NoError(t, mgr.executionStore.Add(exec))

	require.NoError(t, mgr.SetExecutionDescription(context.Background(), "exec-1", "Ship the release"))

	require.Equal(t, "Ship the release", getTaskDescriptionFromMetadata(exec),
		"the description set on a workspace-only execution is what the prompt path reads back")
}

func TestPrepareProgressRecorderMergeFillsOnlyBlankSlots(t *testing.T) {
	recorder := newPrepareProgressRecorder(nil)
	recorder.Callback(0)(PrepareStep{Name: "Validate Docker"}, 0, 2)

	recorder.Merge([]PrepareStep{
		{Name: "Overwrite attempt"},
		{Name: "Create worktree"},
		{Name: "Run setup"},
	})

	steps := recorder.Steps()
	require.Equal(t, 3, recorder.Len())
	require.Equal(t, "Validate Docker", steps[0].Name, "a recorded step must not be overwritten by a merge")
	require.Equal(t, "Create worktree", steps[1].Name, "a blank slot is filled from the merged list")
	require.Equal(t, "Run setup", steps[2].Name, "steps past the recorded length are appended")
}

func TestPrepareProgressRecorderCallbackOffsetsIndexes(t *testing.T) {
	var gotIndex, gotTotal int
	recorder := newPrepareProgressRecorder(func(_ PrepareStep, index, total int) {
		gotIndex, gotTotal = index, total
	})

	recorder.Callback(2)(PrepareStep{Name: "Clone repository"}, 1, 3)

	require.Equal(t, 3, gotIndex, "the callback index is offset by the preceding phase's step count")
	require.Equal(t, 5, gotTotal)
	require.Equal(t, "Clone repository", recorder.Steps()[3].Name)
}

func TestScratchWorkspacePathRejectsTraversalIDs(t *testing.T) {
	mgr := newTestManager(t)
	mgr.dataDir = t.TempDir()

	require.Equal(t, filepath.Join(mgr.dataDir, "tasks", "ws-1", "task-1"),
		mgr.scratchWorkspacePath(&LaunchRequest{TaskID: "task-1", WorkspaceID: "ws-1"}))
	require.Equal(t, filepath.Join(mgr.dataDir, "quick-chat", "session-1"),
		mgr.scratchWorkspacePath(&LaunchRequest{SessionID: "session-1", IsEphemeral: true}))

	require.Empty(t, mgr.scratchWorkspacePath(&LaunchRequest{SessionID: "a/../b", IsEphemeral: true}))
	require.Empty(t, mgr.scratchWorkspacePath(&LaunchRequest{TaskID: "task-1"}),
		"a non-ephemeral scratch workspace needs both a task and a workspace ID")
	require.Empty(t, mgr.scratchWorkspacePath(&LaunchRequest{TaskID: "..", WorkspaceID: "ws-1"}))
	require.Empty(t, mgr.scratchWorkspacePath(&LaunchRequest{TaskID: "task-1", WorkspaceID: "a/b"}))
}

func TestResolveWorkspaceFromProvider(t *testing.T) {
	ctx := context.Background()
	req := &LaunchRequest{TaskID: "task-1", SessionID: "session-1"}

	t.Run("no provider", func(t *testing.T) {
		mgr := newTestManager(t)
		require.Empty(t, mgr.resolveWorkspaceFromProvider(ctx, req))
	})

	t.Run("provider returns path", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
			"session-1": {SessionID: "session-1", WorkspacePath: "/work/task-1"},
		}}
		require.Equal(t, "/work/task-1", mgr.resolveWorkspaceFromProvider(ctx, req))
	})

	t.Run("provider returns empty path", func(t *testing.T) {
		mgr := newTestManager(t)
		mgr.workspaceInfoProvider = &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
			"session-1": {SessionID: "session-1"},
		}}
		require.Empty(t, mgr.resolveWorkspaceFromProvider(ctx, req))
	})
}
