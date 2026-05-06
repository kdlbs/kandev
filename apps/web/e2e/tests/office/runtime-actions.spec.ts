import { test, expect } from "../../fixtures/office-fixture";

test.describe("Office runtime actions", () => {
  test("posts comments, updates status, and creates child tasks under the originating run", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Runtime Happy Path", {
      workflow_id: officeSeed.workflowId,
      agent_profile_id: officeSeed.agentId,
    });
    const seededRun = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "claimed",
      reason: "task_assigned",
      taskId: task.id,
      sessionId: "runtime-actions-session",
    });
    const capabilities = JSON.stringify({
      post_comment: true,
      update_task_status: true,
      create_subtask: true,
      allowed_task_ids: [task.id],
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: seededRun.run_id,
      taskId: task.id,
      sessionId: "runtime-actions-session",
      capabilities,
    });

    const comment = await apiClient.runtimePostComment(
      token,
      task.id,
      "runtime happy-path comment",
    );
    expect(comment.status).toBe(201);
    const status = await apiClient.runtimeUpdateTaskStatus(token, task.id, "in_review");
    expect(status.status).toBe(200);
    const child = await apiClient.runtimeCreateSubtask(token, task.id, {
      title: "Runtime child task",
      description: "Created through runtime syscall",
      assigneeAgentId: officeSeed.agentId,
    });
    expect(child.status).toBe(201);

    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByText("runtime happy-path comment")).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByTestId("status-picker-trigger")).toContainText("In Review");
    await expect(testPage.getByTestId("sub-issues-list")).toContainText("Runtime child task");

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs/${seededRun.run_id}`);
    await expect(testPage.getByTestId("events-log")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("runtime.action").first()).toBeVisible();
    await expect(testPage.getByText(/post_comment/)).toBeVisible();
    await expect(testPage.getByText(/update_task_status/)).toBeVisible();
    await expect(testPage.getByText(/create_subtask/)).toBeVisible();
  });
});
