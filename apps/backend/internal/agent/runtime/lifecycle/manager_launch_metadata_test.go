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
