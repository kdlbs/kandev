import { expect, test } from "../../fixtures/ssh-test-base";
import { readRemoteFile } from "../../helpers/ssh";
import {
  disableNativeCodexAppServer,
  createNativeCodexExecutorProbe,
  expectNativeCodexLaunchBlocked,
  launchNativeCodexProbe,
  removeNativeCodexExecutorProbe,
} from "../../helpers/native-codex-app-server-executor";

test("native Codex profile is gated and launches through the SSH executor", async ({
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(240_000);
  const probe = await createNativeCodexExecutorProbe(backend, apiClient, "E2E SSH Codex");
  const executorProfile = await apiClient.createExecutorProfile(seedData.sshExecutorId, {
    name: "E2E SSH native Codex",
    config: {},
    prepare_script: `workspace={{workspace.path}}
repository_url={{repository.clone_url}}
repository_branch={{repository.branch}}
git init -q "$workspace"
git -C "$workspace" remote add origin "$repository_url"
git -C "$workspace" fetch --no-tags origin "+refs/heads/$repository_branch:refs/remotes/origin/$repository_branch"
git -C "$workspace" checkout -b "$repository_branch" "origin/$repository_branch"
${probe.prepareScript}`,
    cleanup_script: "",
    env_vars: [],
  });
  const seed = {
    workspaceId: seedData.workspaceId,
    workflowId: seedData.workflowId,
    startStepId: seedData.startStepId,
    executorId: seedData.sshExecutorId,
    executorProfileId: executorProfile.id,
    repositoryId: seedData.repositoryId,
  };

  try {
    await launchNativeCodexProbe(
      apiClient,
      seed,
      probe.profile.id,
      "SSH native Codex command launch",
    );
    expect(readRemoteFile(seedData.sshTarget, probe.markerPath)).toMatch(
      /@openai\/codex@0\.154\.0 app-server/,
    );

    await disableNativeCodexAppServer(backend, apiClient, probe.profile.id);
    const disabledError = await expectNativeCodexLaunchBlocked(apiClient, seed, probe.profile.id);
    expect(disabledError).toMatch(/codex|agent|profile|disabled/i);
  } finally {
    await apiClient.deleteExecutorProfile(executorProfile.id).catch(() => undefined);
    await removeNativeCodexExecutorProbe(backend, apiClient, probe.profile.id);
  }
});
