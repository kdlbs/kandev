import {
  test,
  expect,
  kubernetesExecutorConfig,
  kubernetesProfileConfig,
} from "../../fixtures/kubernetes-test-base";
import type { KubernetesPod } from "../../fixtures/kubernetes-tools";
import {
  execInKubernetesPod,
  waitForKubernetesPod,
  waitForKubernetesPVC,
  waitForKubernetesResourceAbsent,
  waitForTaskSessionState,
} from "../../helpers/kubernetes";
import {
  waitForAgentMessage,
  waitForLatestSessionDone,
  waitForSessionDone,
} from "../../helpers/session";

test("sessions share one task pod through stop and backend restart", async ({
  apiClient,
  seedData,
  cluster,
  backend,
}) => {
  test.setTimeout(360_000);
  await apiClient.updateExecutor(seedData.executorId, {
    config: kubernetesExecutorConfig(cluster, { request_timeout_seconds: "120" }),
  });
  const profile = await apiClient.createExecutorProfile(seedData.executorId, {
    name: "Shared task pod",
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
    config: kubernetesProfileConfig(cluster, {
      "workspace.mode": "managed_pvc",
      "workspace.size": "1Gi",
      "workspace.access_modes": JSON.stringify(["ReadWriteOnce"]),
    }),
  });
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Shared Kubernetes task",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      executor_id: seedData.executorId,
      executor_profile_id: profile.id,
    },
  );
  await waitForLatestSessionDone(apiClient, task.id, 1, "First session ready", 180_000);
  expect(task.session_id).toBeTruthy();
  const pod = await waitForKubernetesPod(cluster, task.id, task.session_id!);
  const claim = await waitForKubernetesPVC(cluster, task.id, task.session_id!);
  execInKubernetesPod(cluster, pod.metadata.name, [
    "sh",
    "-c",
    "printf shared > /workspace/shared-marker",
  ]);
  const launchAdditional = () =>
    apiClient.launchSession(
      {
        task_id: task.id,
        agent_profile_id: seedData.agentProfileId,
        executor_id: seedData.executorId,
        executor_profile_id: profile.id,
        prompt: "/e2e:simple-message",
      },
      90_000,
    );
  const [second, third] = await Promise.all([launchAdditional(), launchAdditional()]);
  await waitForSessionDone(apiClient, task.id, second.session_id, "Second session ready");
  await waitForSessionDone(apiClient, task.id, third.session_id, "Concurrent sibling ready");
  const taskPods = () =>
    cluster.json<{ items: KubernetesPod[] }>([
      "-n",
      cluster.namespace,
      "get",
      "pods",
      "-l",
      `kandev.ai/task-id=${task.id}`,
    ]).items;
  expect(taskPods()).toHaveLength(1);
  expect(taskPods()[0].metadata.uid).toBe(pod.metadata.uid);
  const rows = (await apiClient.listKubernetesSessions(seedData.executorId)).filter(
    (row) => row.task_id === task.id,
  );
  expect(rows).toHaveLength(3);
  await test.info().attach("shared-task-session-status", {
    body: JSON.stringify(rows, null, 2),
    contentType: "application/json",
  });
  const instances = execInKubernetesPod(cluster, pod.metadata.name, [
    "sh",
    "-c",
    "find /run/kandev/sessions -name auth.env | wc -l",
  ]);
  expect(Number(instances.trim())).toBe(3);
  await apiClient.stopSession({ session_id: task.session_id!, force: true });
  await waitForTaskSessionState(apiClient, task.id, task.session_id!, "CANCELLED");
  expect(taskPods()).toHaveLength(1);
  await apiClient.addUserMessage(
    task.id,
    second.session_id,
    'e2e:message("Sibling survived stop")',
  );
  await waitForAgentMessage(apiClient, second.session_id, "Sibling survived stop");
  await waitForSessionDone(apiClient, task.id, second.session_id, "Sibling survives stop");
  await backend.restart();
  await apiClient.addUserMessage(
    task.id,
    second.session_id,
    'e2e:message("Sibling survived restart")',
  );
  await waitForAgentMessage(apiClient, second.session_id, "Sibling survived restart");
  await waitForSessionDone(
    apiClient,
    task.id,
    second.session_id,
    "Sibling survives backend restart",
  );
  expect(taskPods()).toHaveLength(1);
  expect(taskPods()[0].metadata.uid).toBe(pod.metadata.uid);
  expect(execInKubernetesPod(cluster, pod.metadata.name, ["cat", "/workspace/shared-marker"])).toBe(
    "shared",
  );
  await apiClient.archiveTask(task.id);
  await waitForKubernetesResourceAbsent(cluster, "pod", pod.metadata.name);
  await waitForKubernetesResourceAbsent(cluster, "persistentvolumeclaim", claim.metadata.name);
  expect(rows.map((row) => ({ pod: row.pod_name, failure: row.failure_reason ?? "" }))).toEqual(
    rows.map(() => ({ pod: pod.metadata.name, failure: "" })),
  );
});
