import { expect, kubernetesProfileConfig, test } from "../../fixtures/kubernetes-test-base";
import { execInKubernetesPod, waitForKubernetesPod } from "../../helpers/kubernetes";
import {
  disableNativeCodexAppServer,
  createNativeCodexExecutorProbe,
  expectNativeCodexLaunchBlocked,
  launchNativeCodexProbe,
  removeNativeCodexExecutorProbe,
} from "../../helpers/native-codex-app-server-executor";

test("native Codex profile is gated and launches through the Kubernetes executor", async ({
  apiClient,
  backend,
  cluster,
  seedData,
}) => {
  test.setTimeout(360_000);
  const probe = await createNativeCodexExecutorProbe(backend, apiClient, "E2E Kubernetes Codex");
  const executorProfile = await apiClient.createExecutorProfile(seedData.executorId, {
    name: "E2E Kubernetes native Codex",
    config: kubernetesProfileConfig(cluster),
    prepare_script: probe.prepareScript,
    cleanup_script: "",
    env_vars: [],
  });
  const seed = {
    workspaceId: seedData.workspaceId,
    workflowId: seedData.workflowId,
    startStepId: seedData.startStepId,
    executorId: seedData.executorId,
    executorProfileId: executorProfile.id,
  };

  try {
    const taskId = await launchNativeCodexProbe(
      apiClient,
      seed,
      probe.profile.id,
      "Kubernetes native Codex command launch",
    );
    const { sessions } = await apiClient.listTaskSessions(taskId);
    const sessionId = sessions[0]?.id;
    expect(sessionId).toBeTruthy();
    const pod = await waitForKubernetesPod(cluster, taskId, sessionId!);
    expect(execInKubernetesPod(cluster, pod.metadata.name, ["cat", probe.markerPath])).toMatch(
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
