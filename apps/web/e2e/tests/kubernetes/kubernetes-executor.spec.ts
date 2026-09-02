import type { Page } from "@playwright/test";
import {
  kubernetesExecutorConfig,
  kubernetesProfileConfig,
  seedKubernetesBackend,
  test,
  expect,
  type KubernetesSeedData,
} from "../../fixtures/kubernetes-test-base";
import {
  processTcpListeners,
  type KubernetesCluster,
  type KubernetesPod,
} from "../../fixtures/kubernetes-tools";
import { ApiClient } from "../../helpers/api-client";
import {
  execInKubernetesPod,
  kubernetesContainerLogsContainCredential,
  readKubeconfigBearerToken,
  recreateForeignPod,
  waitForFailedSession,
  waitForKubernetesPod,
  waitForKubernetesPVC,
  waitForKubernetesResourceAbsent,
  waitForKubernetesRestart,
  waitForTaskResourceCleanupAttempt,
  waitForTaskSessionState,
} from "../../helpers/kubernetes";
import {
  waitForAgentMessage,
  waitForLatestSessionDone,
  waitForSessionDone,
} from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

function launchTask(
  apiClient: ApiClient,
  seed: KubernetesSeedData,
  title: string,
  description = "/e2e:simple-message",
) {
  return apiClient.createTaskWithAgent(seed.workspaceId, title, seed.agentProfileId, {
    description,
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    executor_id: seed.executorId,
    executor_profile_id: seed.executorProfileId,
  });
}

async function assertUserVisibleFailure(page: Page, taskId: string, expected: RegExp) {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad(30_000);
  const banner = session.activeChat().getByTestId("failed-session-banner");
  await expect(banner).toBeVisible({ timeout: 30_000 });
  await expect(banner).toContainText(expected);
}

async function createProfile(
  apiClient: ApiClient,
  executorId: string,
  name: string,
  config: Record<string, string>,
) {
  return apiClient.createExecutorProfile(executorId, {
    name,
    config,
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });
}

const causalFailureCases: Array<{
  name: string;
  executorConfig?: (cluster: KubernetesCluster) => Record<string, string>;
  profileConfig: (cluster: KubernetesCluster) => Record<string, string>;
  expected: RegExp;
}> = [
  {
    name: "RBAC",
    executorConfig: (cluster) =>
      kubernetesExecutorConfig(cluster, {
        kubeconfig_path: cluster.restrictedKubeconfig,
        request_timeout_seconds: "10",
      }),
    profileConfig: (cluster) => kubernetesProfileConfig(cluster),
    expected: /forbidden|cannot create.*pods|RBAC/i,
  },
  {
    name: "scheduling",
    profileConfig: (cluster) =>
      kubernetesProfileConfig(cluster, {
        pod_template_yaml: cluster.podTemplate({
          nodeSelector: { "kandev.ai/e2e-never": "true" },
        }),
      }),
    expected: /schedul|didn't match|0\/1 nodes/i,
  },
  {
    name: "image pull",
    profileConfig: (cluster) =>
      kubernetesProfileConfig(cluster, {
        pod_template_yaml: cluster.podTemplate({
          image: "registry.invalid/kandev/e2e-missing:never",
          imagePullPolicy: "Always",
        }),
      }),
    expected: /ImagePullBackOff|ErrImagePull|image pull/i,
  },
  {
    name: "PVC",
    profileConfig: (cluster) =>
      kubernetesProfileConfig(cluster, {
        "workspace.mode": "existing_claim",
        "workspace.claim_name": "kandev-e2e-missing-claim",
      }),
    expected: /persistentvolumeclaim|PVC|not found/i,
  },
];

test("launches through kubeconfig with Pod exec and a loopback-only agentctl forward", async ({
  apiClient,
  seedData,
  cluster,
  backend,
  testPage,
}) => {
  test.setTimeout(240_000);
  const pid = backend.pid();
  expect(pid).toBeTruthy();
  const listenersBefore = new Set(processTcpListeners(pid!).map((row) => row.port));

  const task = await launchTask(apiClient, seedData, "Kubernetes kubeconfig launch");
  expect(task.session_id).toBeTruthy();
  await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for Kubernetes kubeconfig launch");
  const pod = await waitForKubernetesPod(cluster, task.id, task.session_id!);
  const labels = pod.metadata.labels ?? {};
  expect(labels["app.kubernetes.io/managed-by"]).toBe("kandev");
  expect(labels["kandev.ai/executor-id"]).toBe(seedData.executorId);
  expect(labels["kandev.ai/profile-id"]).toBe(seedData.executorProfileId);
  expect(labels["kandev.ai/task-id"]).toBe(task.id);
  expect(labels["kandev.ai/session-id"]).toBe(task.session_id);
  expect(labels["kandev.ai/instance-id"]).toBeTruthy();
  expect(labels["kandev.ai/environment-id"]).toBeTruthy();

  const exactPod = cluster.json<KubernetesPod>([
    "-n",
    cluster.namespace,
    "get",
    "pod",
    pod.metadata.name,
  ]);
  const hostBearerToken = readKubeconfigBearerToken(cluster.hostKubeconfig);
  const serializedPodSpec = JSON.stringify(exactPod.spec ?? {});
  expect(
    serializedPodSpec.includes(hostBearerToken),
    "the agent Pod spec must not serialize the host kubeconfig bearer credential",
  ).toBe(false);
  expect(exactPod.spec?.automountServiceAccountToken).toBe(false);
  expect(
    kubernetesContainerLogsContainCredential(cluster, exactPod, hostBearerToken),
    "current and previous agent Pod logs must not contain the host kubeconfig bearer credential",
  ).toBe(false);

  expect(
    execInKubernetesPod(cluster, pod.metadata.name, ["test", "-x", "/opt/kandev/agentctl"]),
  ).toBe("");
  const services = cluster.json<{ items: unknown[] }>([
    "-n",
    cluster.namespace,
    "get",
    "services",
    "-l",
    `kandev.ai/task-id=${task.id}`,
  ]);
  expect(services.items).toEqual([]);

  const newListeners = processTcpListeners(pid!).filter((row) => !listenersBefore.has(row.port));
  expect(
    newListeners.length,
    "client-go must retain a local agentctl port-forward",
  ).toBeGreaterThan(0);
  expect(newListeners.every((row) => row.address === "127.0.0.1" || row.address === "::1")).toBe(
    true,
  );

  const sessions = await apiClient.listKubernetesSessions(seedData.executorId);
  expect(sessions.find((row) => row.task_id === task.id)).toMatchObject({
    session_id: task.session_id,
    pod_name: pod.metadata.name,
    workspace_kind: "empty_dir",
  });

  await testPage.goto(`/t/${task.id}`);
  await new SessionPage(testPage).waitForLoad(30_000);
  const executorControl = testPage.getByTestId("executor-settings-button");
  await expect(executorControl).toBeVisible();
  await expect(executorControl.getByTestId("executor-status-kubernetes-icon")).toBeVisible();
  await executorControl.hover();

  const disclosure = testPage.getByTestId("executor-settings-popover");
  await expect(disclosure).toBeVisible();
  await expect(disclosure).toContainText(pod.metadata.name);
  await expect(disclosure).toContainText("Running");
  await expect(disclosure).toContainText("empty_dir");
  await expect(disclosure).toContainText("Restarts");
  await expect(disclosure).not.toContainText("No resource details available.");
  await expect(disclosure.getByTestId("executor-settings-refresh")).toBeVisible();
  await expect(disclosure.getByTestId("executor-settings-reset")).toHaveCount(0);
  const settingsLink = disclosure.getByTestId("executor-settings-link");
  await expect(settingsLink).toHaveAttribute(
    "href",
    `/settings/executors/${seedData.executorProfileId}`,
  );
  await settingsLink.click();
  await expect(testPage).toHaveURL(
    (url) => url.pathname === `/settings/executors/${seedData.executorProfileId}`,
  );
  await expect(testPage.getByTestId("kubernetes-sessions-table")).toContainText(pod.metadata.name);
});

test("launches from a real in-cluster service account", async ({ cluster }) => {
  test.setTimeout(300_000);
  const backend = await cluster.startInClusterBackend();
  const apiClient = new ApiClient(backend.baseUrl);
  const seed = await seedKubernetesBackend(apiClient, cluster, {
    label: "in-cluster",
    executorConfig: kubernetesExecutorConfig(cluster, {
      auth_mode: "in_cluster",
      kubeconfig_path: "",
      kube_context: "",
    }),
  });
  try {
    const result = await apiClient.testKubernetesConnection({
      config: kubernetesExecutorConfig(cluster, {
        auth_mode: "in_cluster",
        kubeconfig_path: "",
        kube_context: "",
      }),
      profile_config: kubernetesProfileConfig(cluster),
    });
    expect(
      result,
      `In-cluster connection result:\n${JSON.stringify(result, null, 2)}`,
    ).toMatchObject({
      success: true,
      namespace: cluster.namespace,
    });

    const task = await launchTask(apiClient, seed, "Kubernetes in-cluster launch");
    await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for in-cluster launch");
    expect(task.session_id).toBeTruthy();
    const pod = await waitForKubernetesPod(cluster, task.id, task.session_id!);
    expect(pod.metadata.labels?.["kandev.ai/executor-id"]).toBe(seed.executorId);
  } finally {
    await apiClient.e2eReset(seed.workspaceId, [seed.workflowId]).catch(() => undefined);
    await apiClient.deleteExecutorProfile(seed.executorProfileId).catch(() => undefined);
    await apiClient.deleteExecutor(seed.executorId).catch(() => undefined);
    await cluster.cleanupWorkloads();
  }
});

test("reconnects to the same Pod after backend restart", async ({
  apiClient,
  seedData,
  cluster,
  backend,
}) => {
  test.setTimeout(300_000);
  const task = await launchTask(apiClient, seedData, "Kubernetes backend reconnect");
  expect(task.session_id).toBeTruthy();
  await waitForLatestSessionDone(apiClient, task.id, 1, "Launch before backend restart");
  const before = await waitForKubernetesPod(cluster, task.id, task.session_id!);
  const beforeMessages = (await apiClient.listSessionMessages(task.session_id!)).messages.length;

  await backend.restart();
  await expect
    .poll(
      async () =>
        (await apiClient.listKubernetesSessions(seedData.executorId)).find(
          (row) => row.task_id === task.id,
        )?.pod_name ?? "",
      { timeout: 90_000, message: "Waiting for Kubernetes reconnect inventory" },
    )
    .toBe(before.metadata.name);

  await apiClient.addUserMessage(task.id, task.session_id!, "/e2e:simple-message");
  await waitForSessionDone(apiClient, task.id, task.session_id!, "Turn after backend reconnect");
  await expect
    .poll(async () => (await apiClient.listSessionMessages(task.session_id!)).messages.length, {
      timeout: 60_000,
      message: "Waiting for post-reconnect agent response",
    })
    .toBeGreaterThan(beforeMessages);
  const after = await waitForKubernetesPod(cluster, task.id, task.session_id!);
  expect(after.metadata.uid).toBe(before.metadata.uid);
});

test("re-handshakes after a live main-container restart", async ({
  apiClient,
  seedData,
  cluster,
}) => {
  test.setTimeout(300_000);
  const task = await launchTask(apiClient, seedData, "Kubernetes container restart");
  expect(task.session_id).toBeTruthy();
  await waitForLatestSessionDone(apiClient, task.id, 1, "Launch before container restart");
  const before = await waitForKubernetesPod(cluster, task.id, task.session_id!);
  const restartCount =
    before.status?.containerStatuses?.find((row) => row.name === "kandev-agent")?.restartCount ?? 0;

  try {
    execInKubernetesPod(cluster, before.metadata.name, ["/bin/sh", "-c", "kill 1"]);
  } catch {
    // Expected: killing PID 1 closes the exec stream before kubectl receives an exit status.
  }
  const restarted = await waitForKubernetesRestart(cluster, before.metadata.name, restartCount);
  expect(restarted.metadata.uid).toBe(before.metadata.uid);

  await apiClient.addUserMessage(task.id, task.session_id!, "/e2e:simple-message");
  await waitForSessionDone(apiClient, task.id, task.session_id!, "Turn after agentctl restart");
  await expect
    .poll(
      async () =>
        (await apiClient.listKubernetesSessions(seedData.executorId)).find(
          (row) => row.task_id === task.id,
        )?.restarts ?? 0,
      { timeout: 60_000, message: "Waiting for restart count in Kubernetes settings status" },
    )
    .toBeGreaterThan(restartCount);
});

test("preserves a managed PVC across ordinary stop/resume and deletes it terminally", async ({
  apiClient,
  seedData,
  cluster,
}) => {
  test.setTimeout(300_000);
  const profile = await createProfile(
    apiClient,
    seedData.executorId,
    "E2E managed PVC",
    kubernetesProfileConfig(cluster, {
      "workspace.mode": "managed_pvc",
      "workspace.size": "1Gi",
      "workspace.access_modes": JSON.stringify(["ReadWriteOnce"]),
    }),
  );
  const managedSeed = { ...seedData, executorProfileId: profile.id };
  try {
    const task = await launchTask(
      apiClient,
      managedSeed,
      "Kubernetes managed PVC retention",
      'e2e:message("started")\ne2e:delay(60000)',
    );
    expect(task.session_id).toBeTruthy();
    const pod = await waitForKubernetesPod(cluster, task.id, task.session_id!);
    const claim = await waitForKubernetesPVC(cluster, task.id, task.session_id!);
    execInKubernetesPod(cluster, pod.metadata.name, [
      "/bin/sh",
      "-c",
      "printf retained > /workspace/kandev-retained",
    ]);
    await waitForAgentMessage(apiClient, task.session_id!, "started");

    await apiClient.stopSession({ session_id: task.session_id! });
    await waitForTaskSessionState(apiClient, task.id, task.session_id!, "CANCELLED");
    const resumed = await apiClient.launchSession(
      { task_id: task.id, intent: "resume", session_id: task.session_id! },
      90_000,
    );
    expect(resumed.session_id).toBe(task.session_id);
    await waitForTaskSessionState(
      apiClient,
      task.id,
      task.session_id!,
      "WAITING_FOR_INPUT",
      90_000,
    );
    const beforeMessages = (await apiClient.listSessionMessages(task.session_id!)).messages.length;
    await apiClient.addUserMessage(task.id, task.session_id!, "/e2e:simple-message");
    await waitForSessionDone(apiClient, task.id, task.session_id!, "Managed PVC resume turn");
    await expect
      .poll(async () => (await apiClient.listSessionMessages(task.session_id!)).messages.length, {
        timeout: 60_000,
        message: "Waiting for the resumed managed-PVC session response",
      })
      .toBeGreaterThan(beforeMessages);
    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions.map((session) => session.id)).toEqual([task.session_id]);
    const after = await waitForKubernetesPod(cluster, task.id, task.session_id!);
    const afterClaim = await waitForKubernetesPVC(cluster, task.id, task.session_id!);
    expect(after.metadata.uid).toBe(pod.metadata.uid);
    expect(afterClaim.metadata.uid).toBe(claim.metadata.uid);
    expect(
      execInKubernetesPod(cluster, after.metadata.name, ["cat", "/workspace/kandev-retained"]),
    ).toBe("retained");

    await apiClient.archiveTask(task.id);
    await waitForKubernetesResourceAbsent(cluster, "pod", pod.metadata.name);
    await waitForKubernetesResourceAbsent(cluster, "persistentvolumeclaim", claim.metadata.name);
  } finally {
    await apiClient.deleteExecutorProfile(profile.id).catch(() => undefined);
  }
});

test("retains an existing claim after terminal task cleanup", async ({
  apiClient,
  seedData,
  cluster,
}) => {
  test.setTimeout(300_000);
  const claimName = "kandev-e2e-existing";
  cluster.kubectl(["apply", "-f", "-"], {
    input: `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${claimName}
  namespace: ${cluster.namespace}
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
`,
  });
  const claimBefore = cluster.json<{ metadata: { uid: string } }>([
    "-n",
    cluster.namespace,
    "get",
    "persistentvolumeclaim",
    claimName,
  ]);
  const profile = await createProfile(
    apiClient,
    seedData.executorId,
    "E2E existing claim",
    kubernetesProfileConfig(cluster, {
      "workspace.mode": "existing_claim",
      "workspace.claim_name": claimName,
    }),
  );
  try {
    const task = await launchTask(
      apiClient,
      { ...seedData, executorProfileId: profile.id },
      "Kubernetes existing claim",
    );
    await waitForLatestSessionDone(apiClient, task.id, 1, "Existing-claim launch");
    expect(task.session_id).toBeTruthy();
    const pod = await waitForKubernetesPod(cluster, task.id, task.session_id!);
    cluster.kubectl([
      "-n",
      cluster.namespace,
      "wait",
      "--for=jsonpath={.status.phase}=Bound",
      `persistentvolumeclaim/${claimName}`,
      "--timeout=90s",
    ]);
    execInKubernetesPod(cluster, pod.metadata.name, [
      "/bin/sh",
      "-c",
      "printf existing-retained > /workspace/existing-retained",
    ]);
    await apiClient.deleteTask(task.id);
    await waitForKubernetesResourceAbsent(cluster, "pod", pod.metadata.name);
    const claimAfter = cluster.json<{ metadata: { uid: string } }>([
      "-n",
      cluster.namespace,
      "get",
      "persistentvolumeclaim",
      claimName,
    ]);
    expect(claimAfter.metadata.uid).toBe(claimBefore.metadata.uid);
  } finally {
    await apiClient.deleteExecutorProfile(profile.id).catch(() => undefined);
  }
});

test("refuses to delete a same-name foreign Pod with a different UID", async ({
  apiClient,
  seedData,
  cluster,
  backend,
}) => {
  test.setTimeout(240_000);
  const task = await launchTask(apiClient, seedData, "Kubernetes UID protection");
  await waitForLatestSessionDone(apiClient, task.id, 1, "UID-protection launch");
  expect(task.session_id).toBeTruthy();
  const original = await waitForKubernetesPod(cluster, task.id, task.session_id!);
  const foreign = recreateForeignPod(cluster, original);
  expect(foreign.metadata.uid).not.toBe(original.metadata.uid);

  await apiClient.archiveTask(task.id);
  const cleanup = await waitForTaskResourceCleanupAttempt(
    backend.tmpDir,
    task.id,
    /runtime stop operations failed/i,
  );
  expect(cleanup.attempts).toBeGreaterThan(0);
  const after = cluster.json<KubernetesPod>([
    "-n",
    cluster.namespace,
    "get",
    "pod",
    original.metadata.name,
  ]);
  expect(after.metadata.uid).toBe(foreign.metadata.uid);
});

test("refuses deletion when any kandev.ai ownership label is foreign", async ({
  apiClient,
  seedData,
  cluster,
  backend,
}) => {
  test.setTimeout(240_000);
  const task = await launchTask(apiClient, seedData, "Kubernetes label protection");
  await waitForLatestSessionDone(apiClient, task.id, 1, "Label-protection launch");
  expect(task.session_id).toBeTruthy();
  const pod = await waitForKubernetesPod(cluster, task.id, task.session_id!);
  cluster.kubectl([
    "-n",
    cluster.namespace,
    "label",
    "pod",
    pod.metadata.name,
    "kandev.ai/foreign-owner=true",
    "--overwrite",
  ]);

  await apiClient.archiveTask(task.id);
  const cleanup = await waitForTaskResourceCleanupAttempt(
    backend.tmpDir,
    task.id,
    /runtime stop operations failed/i,
  );
  expect(cleanup.attempts).toBeGreaterThan(0);
  const after = cluster.json<KubernetesPod>([
    "-n",
    cluster.namespace,
    "get",
    "pod",
    pod.metadata.name,
  ]);
  expect(after.metadata.uid).toBe(pod.metadata.uid);
  expect(after.metadata.labels?.["kandev.ai/foreign-owner"]).toBe("true");
});

for (const failure of causalFailureCases) {
  test(`surfaces causal ${failure.name} failure in user-visible status`, async ({
    apiClient,
    seedData,
    cluster,
    testPage,
  }) => {
    test.setTimeout(180_000);
    const executorConfig = failure.executorConfig?.(cluster);
    const executor = executorConfig
      ? await apiClient.createExecutor(`E2E Kubernetes ${failure.name}`, "k8s", executorConfig)
      : null;
    const executorId = executor?.id ?? seedData.executorId;
    const profile = await createProfile(
      apiClient,
      executorId,
      `E2E ${failure.name}`,
      failure.profileConfig(cluster),
    );
    try {
      const task = await launchTask(
        apiClient,
        { ...seedData, executorId, executorProfileId: profile.id },
        `Kubernetes ${failure.name} failure`,
      );
      await waitForFailedSession(apiClient, task.id, failure.expected, 120_000);
      await assertUserVisibleFailure(testPage, task.id, failure.expected);
      await apiClient.deleteTask(task.id).catch(() => undefined);
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => undefined);
      if (executor) await apiClient.deleteExecutor(executor.id).catch(() => undefined);
      await cluster.cleanupWorkloads();
    }
  });
}
