/**
 * Regression test for kdlbs/kandev#1657: the sidebar "waiting for input"
 * question icon must show for a task blocked on an agent clarification even
 * when the task has never been opened, purely from the task snapshot.
 */
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

async function waitForSessionWaitingForInput(
  apiClient: ApiClient,
  taskId: string,
  message: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        return sessions[0]?.state ?? "";
      },
      { message, timeout: 60_000 },
    )
    .toBe("WAITING_FOR_INPUT");
}

test.describe("Sidebar pending-question indicator without opening the task", () => {
  test("blocked task shows the question icon on a fresh page load; idle task does not", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    const blockedTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Blocked On Question",
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        repository_ids: [seedData.repositoryId],
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );

    const idleTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Finished Quietly",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        repository_ids: [seedData.repositoryId],
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );

    await waitForSessionWaitingForInput(
      apiClient,
      blockedTask.id,
      "blocked task session should park on the clarification",
    );
    await waitForSessionWaitingForInput(
      apiClient,
      idleTask.id,
      "idle task session should finish its turn",
    );

    const navTask = await apiClient.createTask(seedData.workspaceId, "Nav Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${navTask.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    const blockedRow = session.sidebarTaskItem("Blocked On Question");
    await expect(blockedRow).toBeVisible({ timeout: 10_000 });
    await expect(blockedRow.getByTestId("task-state-waiting-for-input")).toBeVisible({
      timeout: 10_000,
    });

    const idleRow = session.sidebarTaskItem("Finished Quietly");
    await expect(idleRow).toBeVisible({ timeout: 10_000 });
    await expect(idleRow.getByTestId("task-state-waiting-for-input")).toHaveCount(0);
    await expect(idleRow.getByTestId("task-state-pending-permission")).toHaveCount(0);
  });
});
