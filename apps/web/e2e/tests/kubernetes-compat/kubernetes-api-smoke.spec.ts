import {
  expect,
  kubernetesExecutorConfig,
  kubernetesProfileConfig,
  test,
} from "../../fixtures/kubernetes-test-base";
import { processTcpListeners, type KubernetesPod } from "../../fixtures/kubernetes-tools";
import {
  execInKubernetesPod,
  kubernetesContainerLogsContainCredential,
  readKubeconfigBearerToken,
  waitForKubernetesPod,
} from "../../helpers/kubernetes";
import { waitForLatestSessionDone } from "../../helpers/session";

test("proves Kubernetes API compatibility and an agentctl round-trip", async ({
  apiClient,
  seedData,
  cluster,
  backend,
}) => {
  test.setTimeout(300_000);

  const result = await apiClient.testKubernetesConnection({
    config: kubernetesExecutorConfig(cluster),
    profile_config: kubernetesProfileConfig(cluster),
  });
  expect(result.success).toBe(true);
  expect(result.server_version).toBe(cluster.kubernetesVersion);
  expect(result.namespace).toBe(cluster.namespace);
  for (const key of ["discovery", "admission.pod", "streaming"]) {
    expect(result.steps.find((step) => step.key === key)).toMatchObject({ success: true });
  }
  expect(result.steps.find((step) => step.key === "streaming")?.detail).toContain(
    "Exec and port-forward transports succeeded",
  );

  const pid = backend.pid();
  expect(pid).toBeTruthy();
  const listenersBefore = new Set(processTcpListeners(pid!).map((listener) => listener.port));
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `Kubernetes ${cluster.kubernetesVersion} compatibility`,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      executor_id: seedData.executorId,
      executor_profile_id: seedData.executorProfileId,
    },
  );
  expect(task.session_id).toBeTruthy();
  await waitForLatestSessionDone(
    apiClient,
    task.id,
    1,
    `Waiting for Kubernetes ${cluster.kubernetesVersion} compatibility launch`,
  );

  const pod = await waitForKubernetesPod(cluster, task.id, task.session_id!);
  expect(
    execInKubernetesPod(cluster, pod.metadata.name, ["test", "-x", "/opt/kandev/agentctl"]),
  ).toBe("");
  const inventory = await apiClient.listKubernetesSessions(seedData.executorId);
  expect(inventory.find((session) => session.session_id === task.session_id)).toMatchObject({
    task_id: task.id,
    pod_name: pod.metadata.name,
  });
  const { messages } = await apiClient.listSessionMessages(task.session_id!);
  expect(messages.some((message) => message.author_type === "agent")).toBe(true);

  const newListeners = processTcpListeners(pid!).filter(
    (listener) => !listenersBefore.has(listener.port),
  );
  expect(newListeners.length).toBeGreaterThan(0);
  expect(
    newListeners.every(
      (listener) => listener.address === "127.0.0.1" || listener.address === "::1",
    ),
  ).toBe(true);

  const exactPod = cluster.json<KubernetesPod>([
    "-n",
    cluster.namespace,
    "get",
    "pod",
    pod.metadata.name,
  ]);
  const credential = readKubeconfigBearerToken(cluster.hostKubeconfig);
  expect(
    JSON.stringify(exactPod.spec ?? {}).includes(credential),
    "agent Pod spec contains host kubeconfig credential: false expected",
  ).toBe(false);
  expect(exactPod.spec?.automountServiceAccountToken).toBe(false);
  expect(
    kubernetesContainerLogsContainCredential(cluster, exactPod, credential),
    "agent Pod logs contain host kubeconfig credential: false expected",
  ).toBe(false);
  expect(
    cluster.diagnostics().includes(credential),
    "redacted Kubernetes diagnostics contain host kubeconfig credential: false expected",
  ).toBe(false);
});
