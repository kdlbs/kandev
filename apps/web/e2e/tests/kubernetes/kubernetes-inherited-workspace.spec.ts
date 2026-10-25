import { test, expect } from "../../fixtures/kubernetes-test-base";
import { watchWs } from "../../helpers/causal-waits";
import { execInKubernetesPod, waitForKubernetesPod } from "../../helpers/kubernetes";
import { waitForLatestSessionDone } from "../../helpers/session";

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4
// @covers AC-TASKS-MCP-WORKSPACE-MODE-004.2
// @covers AC-TASKS-MCP-WORKSPACE-MODE-004.3
test("queued MCP Kubernetes children reject inherited admission and accept new_workspace", async ({
  apiClient,
  seedData,
  cluster,
  testPage,
}) => {
  test.setTimeout(360_000);
  const create = (title: string, workspaceMode?: string) =>
    `e2e:mcp:kandev:create_task_kandev(${JSON.stringify({
      parent_id: "self",
      title,
      start_agent: false,
      external_id: `kubernetes-admission-${title}`,
      prompt: "Queued executor admission regression",
      ...(workspaceMode ? { workspace_mode: workspaceMode } : {}),
    })})`;
  const parent = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Kubernetes admission parent",
    seedData.agentProfileId,
    {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      executor_id: seedData.executorId,
      executor_profile_id: seedData.executorProfileId,
      description: [
        create("Inherited Kubernetes child"),
        create("Independent Kubernetes child", "new_workspace"),
        'e2e:message("Children queued")',
      ].join("\n"),
    },
  );
  await waitForLatestSessionDone(apiClient, parent.id, 1, "Parent ready", 180_000);
  const children = (await apiClient.listTasks(seedData.workspaceId)).tasks;
  const inherited = children.filter((task) => task.title === "Inherited Kubernetes child");
  const independent = children.filter((task) => task.title === "Independent Kubernetes child");
  expect(inherited).toHaveLength(1);
  expect(independent).toHaveLength(1);
  for (const child of [inherited[0], independent[0]]) {
    const stored = await apiClient.getTask(child.id);
    expect(stored.parent_id).toBe(parent.id);
    expect(stored.metadata?.executor_profile_id).toBe(seedData.executorProfileId);
    expect((await apiClient.listTaskSessions(child.id)).sessions).toHaveLength(0);
  }
  expect((await apiClient.getTask(inherited[0].id)).metadata?.workspace).toMatchObject({
    mode: "inherit_parent",
  });
  expect((await apiClient.getTask(independent[0].id)).metadata?.workspace).toMatchObject({
    mode: "new_workspace",
  });
  const original = await apiClient.getTaskEnvironment(parent.id);
  expect(original).toMatchObject({ task_id: parent.id, executor_type: "k8s", status: "ready" });
  const pod = await waitForKubernetesPod(cluster, parent.id, parent.session_id!);
  execInKubernetesPod(cluster, pod.metadata.name, [
    "sh",
    "-c",
    "printf preserved > /workspace/dirty-admission-marker",
  ]);

  const ws = watchWs(testPage);
  const admission = ws.waitForResponse("session.ensure").then(
    () => null,
    (error: Error) => error,
  );
  await testPage.goto(`/t/${inherited[0].id}`);
  const rejected = await admission;
  expect(rejected).toBeInstanceOf(Error);
  expect(rejected?.message).toContain('"code":"CONFLICT"');
  expect(rejected?.message).toContain('"reason":"workspace_reuse_unsupported"');
  const banner = testPage.getByTestId("ensure-session-error-banner");
  await expect(banner).toBeVisible();
  await banner.locator("summary").click();
  await expect(banner.locator("pre")).toBeVisible();
  await expect(banner.locator("pre")).toContainText("new_workspace");
  expect((await apiClient.listTaskSessions(inherited[0].id)).sessions).toHaveLength(0);
  expect(await apiClient.getTaskEnvironment(inherited[0].id)).toBeNull();

  const prepared = await apiClient.launchSession(
    {
      task_id: independent[0].id,
      intent: "prepare",
      launch_workspace: true,
      agent_profile_id: seedData.agentProfileId,
    },
    120_000,
  );
  await expect
    .poll(() => apiClient.getTaskEnvironment(independent[0].id), { timeout: 180_000 })
    .toMatchObject({
      task_id: independent[0].id,
      executor_type: "k8s",
      executor_profile_id: seedData.executorProfileId,
      status: "ready",
    });
  const own = await apiClient.getTaskEnvironment(independent[0].id);
  expect(own?.id).not.toBe(original?.id);
  const childPod = await waitForKubernetesPod(cluster, independent[0].id, prepared.session_id);
  expect(childPod.metadata.uid).not.toBe(pod.metadata.uid);
  expect((await apiClient.listTaskSessions(independent[0].id)).sessions).toEqual([
    expect.objectContaining({ id: prepared.session_id, state: "CREATED" }),
  ]);
  expect(await apiClient.getTaskEnvironment(parent.id)).toEqual(original);
  const stillRunning = await waitForKubernetesPod(cluster, parent.id, parent.session_id!);
  expect(stillRunning.metadata.uid).toBe(pod.metadata.uid);
  expect(
    execInKubernetesPod(cluster, pod.metadata.name, ["cat", "/workspace/dirty-admission-marker"]),
  ).toBe("preserved");
});
