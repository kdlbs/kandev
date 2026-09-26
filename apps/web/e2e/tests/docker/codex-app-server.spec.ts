import { expect, test } from "../../fixtures/docker-test-base";
import { E2E_IMAGE_TAG } from "../../fixtures/docker-probe";
import { dockerFileContent } from "../../helpers/docker";
import {
  disableNativeCodexAppServer,
  createNativeCodexExecutorProbe,
  expectNativeCodexLaunchBlocked,
  launchNativeCodexProbe,
  removeNativeCodexExecutorProbe,
} from "../../helpers/native-codex-app-server-executor";

test("native Codex profile is gated and launches through the Docker executor", async ({
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(240_000);
  const probe = await createNativeCodexExecutorProbe(backend, apiClient, "E2E Docker Codex");
  const { executors } = await apiClient.listExecutors();
  const executor = executors.find((candidate) => candidate.type === "local_docker");
  expect(executor?.id).toBeTruthy();
  const executorProfile = await apiClient.createExecutorProfile(executor!.id, {
    name: "E2E Docker native Codex",
    config: { image_tag: E2E_IMAGE_TAG },
    prepare_script: probe.prepareScript,
    cleanup_script: "",
    env_vars: seedData.gitConfigEnvVars,
  });
  const seed = {
    workspaceId: seedData.workspaceId,
    workflowId: seedData.workflowId,
    startStepId: seedData.startStepId,
    executorId: executor!.id,
    executorProfileId: executorProfile.id,
    repositoryId: seedData.repositoryId,
  };

  try {
    const taskId = await launchNativeCodexProbe(
      apiClient,
      seed,
      probe.profile.id,
      "Docker native Codex command launch",
    );
    const environment = await apiClient.getTaskEnvironment(taskId);
    expect(environment?.executor_type).toBe("local_docker");
    expect(environment?.container_id).toBeTruthy();
    expect(dockerFileContent(environment!.container_id!, probe.markerPath)).toMatch(
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
