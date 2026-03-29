import { test, expect } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";

test.describe("Tool completion on turn end", () => {
  test("tool groups have no spinner after turn completes with uncompleted tool calls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Use tool_use WITHOUT tool_result — leaves tools in "pending" status.
    // The safety net should mark them as complete when the turn ends.
    const script = [
      'e2e:tool_use("Terminal", {"command": "echo test1"})',
      'e2e:tool_use("Terminal", {"command": "echo test2"})',
      'e2e:message("done")',
    ].join("\n");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Tool Completion Test",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // After turn completes, no grid spinners should remain in the chat
    const spinners = session.chat.locator('[role="status"][aria-label="Loading"]');
    await expect(spinners).toHaveCount(0);
  });
});
