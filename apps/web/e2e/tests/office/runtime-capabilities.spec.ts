import { test, expect } from "../../fixtures/office-fixture";

test.describe("Office runtime capabilities", () => {
  test("denies out-of-scope task status updates and records a run event", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const taskA = await apiClient.createTask(officeSeed.workspaceId, "Runtime Scope A", {
      workflow_id: officeSeed.workflowId,
    });
    const taskB = await apiClient.createTask(officeSeed.workspaceId, "Runtime Scope B", {
      workflow_id: officeSeed.workflowId,
    });
    const seededRun = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "claimed",
      reason: "task_assigned",
      taskId: taskA.id,
      sessionId: "runtime-scope-session",
    });
    const capabilities = JSON.stringify({
      update_task_status: true,
      allowed_task_ids: [taskA.id],
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: seededRun.run_id,
      taskId: taskA.id,
      sessionId: "runtime-scope-session",
      capabilities,
    });

    const response = await apiClient.runtimeUpdateTaskStatus(token, taskB.id, "in_review");

    expect(response.status).toBe(403);
    const unchanged = (await officeApi.getTask(taskB.id)) as { task: { status: string } };
    expect(unchanged.task.status).not.toBe("in_review");

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs/${seededRun.run_id}`);
    await expect(testPage.getByTestId("events-log")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("runtime.denied")).toBeVisible();
    await expect(testPage.getByText(/update_task_status/)).toBeVisible();
  });
});
