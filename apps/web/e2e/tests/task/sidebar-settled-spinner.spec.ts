/**
 * Regression test for the sidebar "stuck generating" spinner: when a task's
 * session settles out of RUNNING (agent turn completes -> WAITING_FOR_INPUT,
 * task -> COMPLETED), the sidebar row must stop showing the yellow generating
 * spinner (`task-state-running`), purely from the task snapshot and without ever
 * opening the completed task.
 *
 * The bug (backend): the turn-activity record was detached while the session was
 * still RUNNING, so the recomputed task-level foreground_activity aggregate cached
 * "generating"; the session then settled with only a session-level state_changed
 * event, so the task aggregate was never republished and the spinner kept spinning.
 * Fixed by republishing the task aggregate when a session leaves RUNNING.
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

test.describe("Sidebar spinner clears when a session settles", () => {
  test("completed task drops the generating spinner without opening it", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    const settledTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Settled Turn",
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
      settledTask.id,
      "settled task session should finish its turn",
    );

    // Navigate to a separate task so the sidebar renders the settled task's row
    // from the task snapshot alone — the completed task is never opened.
    const navTask = await apiClient.createTask(seedData.workspaceId, "Nav Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${navTask.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    const settledRow = session.sidebarTaskItem("Settled Turn");
    await expect(settledRow).toBeVisible({ timeout: 10_000 });

    // The core assertion: the generating spinner must clear once the session
    // settles. It must stay cleared (guarding against a late "generating"
    // task.updated re-appearing), so assert it never re-mounts.
    await expect(settledRow.getByTestId("task-state-running")).toHaveCount(0, {
      timeout: 10_000,
    });
    await expect(settledRow.getByTestId("task-state-running")).toHaveCount(0);

    // A settled turn resolves to a done affordance, not a spinner. `simple-message`
    // parks the session in WAITING_FOR_INPUT with no pending clarification, so the
    // row buckets into "review" and shows the green review check.
    await expect(settledRow.getByTestId("task-state-review")).toBeVisible({ timeout: 10_000 });
  });
});
