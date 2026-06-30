import { test, expect } from "../../fixtures/office-fixture";

test.describe("Office comment input on mobile", () => {
  test("projectless completed office task keeps the comment input visible", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(
      officeSeed.workspaceId,
      "Mobile completed comment input task",
      { workflow_id: officeSeed.workflowId },
    );
    await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${task.id}`, {
      assignee_agent_profile_id: officeSeed.agentId,
    });
    await apiClient.seedTaskSession(task.id, {
      state: "COMPLETED",
      agentProfileId: officeSeed.agentId,
      startedAt: new Date(Date.now() - 30_000).toISOString(),
      completedAt: new Date(Date.now() - 5_000).toISOString(),
    });

    await testPage.goto(`/office/tasks/${task.id}`);

    await expect(
      testPage.getByRole("heading", { name: /Mobile completed comment input task/i }),
    ).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByPlaceholder(/add a comment/i)).toBeVisible({ timeout: 10_000 });
  });
});
