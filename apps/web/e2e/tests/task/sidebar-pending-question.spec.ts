/**
 * Regression test for kdlbs/kandev#1657: the sidebar "waiting for input"
 * question icon must show for a task blocked on an agent clarification even
 * when the task has never been opened — i.e. purely from the task snapshot,
 * without the session's messages in the store.
 *
 * The old behavior derived the icon exclusively from loaded messages, so on a
 * fresh page load a blocked task was indistinguishable from a finished one
 * until the user clicked it.
 */
import { test, expect } from "../../fixtures/test-base";
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
      { timeout: 60_000, message },
    )
    .toBe("WAITING_FOR_INPUT");
}

test.describe("Sidebar pending-question indicator without opening the task", () => {
  test("blocked task shows the question icon on a fresh page load; idle task does not", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // A task whose agent parks on a pending clarification. Never navigated to,
    // so its messages are never loaded client-side.
    const blockedTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Blocked On Question",
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // A control task that finishes its turn normally — its session also ends
    // at WAITING_FOR_INPUT, but with no pending request it must NOT alert.
    const idleTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Finished Quietly",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
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

    // Fresh page load on an unrelated task: both seeded tasks stay unopened,
    // so their pending state can only come from the task snapshot.
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
