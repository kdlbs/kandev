import {
  test,
  expect,
  kubernetesExecutorConfig,
  kubernetesProfileConfig,
} from "../../fixtures/kubernetes-test-base";
import {
  execInKubernetesPod,
  waitForKubernetesPod,
  waitForKubernetesPVC,
  waitForKubernetesResourceAbsent,
} from "../../helpers/kubernetes";
import { waitForAgentMessage } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import type { ExecutorFailureEpisode } from "../../../lib/types/executor-failure";

test.describe.configure({ retries: 0 });
// @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.1, .3, .5, .7, .8, .13
// Exact eviction messages are covered by backend fixtures. This case terminates
// only its owned Pod without depending on node storage-accounting intervals.
test("retains workspace and durable missing Pod evidence after backend restart", async ({
  cluster,
  backend,
  apiClient,
  seedData,
  testPage,
}) => {
  test.setTimeout(720_000);
  // Large cold imports can interrupt control-plane leases. Await recovery before launch.
  await expect
    .poll(
      () => {
        const { items } = JSON.parse(
          cluster.kubectl(["get", "pods", "-n", "kube-system", "-o", "json"]),
        ) as {
          items: Array<{
            metadata: { labels?: Record<string, string> };
            status: { conditions?: Array<{ type: string; status: string }> };
          }>;
        };
        const controllers = items.filter((pod) =>
          ["kube-scheduler", "kube-controller-manager"].includes(
            pod.metadata.labels?.component ?? "",
          ),
        );
        return (
          controllers.length === 2 &&
          controllers.every((pod) =>
            pod.status.conditions?.some(
              (condition) => condition.type === "Ready" && condition.status === "True",
            ),
          )
        );
      },
      { timeout: 240_000 },
    )
    .toBe(true);
  // Cold local-path provisioning shares the Pod readiness deadline.
  await apiClient.updateExecutor(seedData.executorId, {
    config: kubernetesExecutorConfig(cluster, { request_timeout_seconds: "180" }),
  });
  const profile = await apiClient.createExecutorProfile(seedData.executorId, {
    name: "Executor loss visibility",
    config: kubernetesProfileConfig(cluster, {
      "workspace.mode": "managed_pvc",
      "workspace.size": "1Gi",
      "workspace.access_modes": JSON.stringify(["ReadWriteOnce"]),
    }),
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
  });
  const { settings } = await apiClient.getUserSettings();
  await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
  try {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Isolated executor loss",
      seedData.agentProfileId,
      {
        description: "/e2e:cancel-hold executor-failure-initial-accepted",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        executor_id: seedData.executorId,
        executor_profile_id: profile.id,
      },
    );
    const sessionId = task.session_id!;
    expect(sessionId).toBeTruthy();
    try {
      await waitForAgentMessage(apiClient, sessionId, "executor-failure-initial-accepted", 210_000);
    } catch (error) {
      await test.info().attach("executor-startup-diagnostics", {
        body: Buffer.from(cluster.diagnostics()),
        contentType: "text/plain",
      });
      const { sessions } = await apiClient.listTaskSessions(task.id);
      const diagnostics = sessions.map((candidate) => ({
        state: candidate.state,
        error: candidate.error_message,
      }));
      throw new Error(`Provider acceptance failed: ${JSON.stringify(diagnostics)}`, {
        cause: error,
      });
    }
    const pod = await waitForKubernetesPod(cluster, task.id, sessionId);
    const pvc = await waitForKubernetesPVC(cluster, task.id, sessionId);
    execInKubernetesPod(cluster, pod.metadata.name, [
      "/bin/sh",
      "-c",
      "printf retained > /workspace/executor-failure-sentinel; sync",
    ]);
    const initialMessages = await apiClient.listSessionMessages(sessionId);
    const initialReplies = initialMessages.messages
      .filter((message) => message.author_type === "agent" && message.type !== "status")
      .map((message) => message.id);
    expect(initialReplies.length).toBeGreaterThan(0);
    cluster.kubectl([
      "delete",
      "pod",
      pod.metadata.name,
      "-n",
      cluster.namespace,
      "--grace-period=0",
      "--force",
      "--wait=true",
    ]);
    await waitForKubernetesResourceAbsent(cluster, "pod", pod.metadata.name);
    async function episode(): Promise<ExecutorFailureEpisode | undefined> {
      const { tasks } = await apiClient.listTasks(seedData.workspaceId);
      return (
        tasks.find((candidate) => candidate.id === task.id)?.status_summary?.executor_failure ??
        undefined
      );
    }
    await expect
      .poll(async () => (await episode())?.observation.reason, { timeout: 90_000 })
      .toBe("PodNotFound");
    const before = await episode();
    expect(before?.observation.workspace).toBe("retained");
    expect(before?.observation.outcome).toBe("missing");
    expect(before?.observation.message ?? "").not.toMatch(/out of memory/i);
    await backend.restart();
    await expect.poll(async () => (await episode())?.id, { timeout: 90_000 }).toBe(before!.id);
    const retained = await waitForKubernetesPVC(cluster, task.id, sessionId);
    expect(retained.metadata.uid).toBe(pvc.metadata.uid);
    // Reading status and returning to the task must not replace the failed Pod or replay a prompt.
    await waitForKubernetesResourceAbsent(cluster, "pod", pod.metadata.name);
    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${task.id}`);
    await session.waitForLoad();
    const card = testPage.getByTestId("session-executor-failure-card");
    await expect(card).toContainText("Executor no longer available");
    await card.getByTestId("executor-failure-expand").click();
    const details = testPage.getByTestId("executor-failure-details");
    await details.locator("summary").click();
    await expect(details).toContainText("PodNotFound");
    await testPage.getByTestId("executor-recheck").click();
    await expect(testPage.getByTestId("executor-recheck")).toBeEnabled();
    await expect(card).toBeVisible();
    expect((await waitForKubernetesPVC(cluster, task.id, sessionId)).metadata.uid).toBe(
      pvc.metadata.uid,
    );
    await waitForKubernetesResourceAbsent(cluster, "pod", pod.metadata.name);
    const finalMessages = await apiClient.listSessionMessages(sessionId);
    expect(
      finalMessages.messages
        .filter((message) => message.author_type === "agent" && message.type !== "status")
        .map((message) => message.id),
    ).toEqual(initialReplies);
  } finally {
    await apiClient.saveUserSettings({
      prevent_auto_start_agent_on_open: settings.prevent_auto_start_agent_on_open,
    });
  }
});
