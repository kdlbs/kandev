import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { watchWs } from "../../helpers/causal-waits";

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
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const ws = watchWs(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    await Promise.all([
      ws.waitForEvent("session.turn.completed", {
        where: (payload) => payload.session_id === task.session_id,
      }),
      session.sendMessageViaButton(script),
    ]);
    const groupToggle = session.chat.getByRole("button", { name: /2 tool calls/ });
    const toolGroups = groupToggle.locator("..");
    await expect(groupToggle).toBeVisible();
    const spinners = toolGroups.locator('[role="status"][aria-label="Loading"]');
    await expect(spinners).toHaveCount(0);
    await groupToggle.click();
    const terminalTools = toolGroups.getByText("Terminal", { exact: true });
    await expect(terminalTools).toHaveCount(2);
    await expect(terminalTools.nth(0)).toBeVisible();
    await expect(terminalTools.nth(1)).toBeVisible();
    await expect(spinners).toHaveCount(0);
  });
});
