import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const TASK_TITLE = "Secondary Session Status Task";

/**
 * Regression: when a task already has a primary session that is idle
 * ("Turn Finished") and the user opens a second chat tab, the sidebar must
 * reflect the new session's RUNNING state. Before the fix the sidebar read
 * the primary session's state only, so secondary tabs running in the
 * background left the task showing "Turn Finished".
 */
test.describe("Sidebar status with secondary session", () => {
  test("sidebar moves to Running when a non-primary chat tab starts", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // 1. Create task whose first session completes quickly.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      TASK_TITLE,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for the first session to reach Turn Finished.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Navigate to the task.
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle(TASK_TITLE);
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Sanity: sidebar shows the task as "Turn Finished" before we start
    //    the secondary session.
    await expect(
      session.taskInSection(TASK_TITLE, "Turn Finished"),
    ).toBeVisible({ timeout: 15_000 });

    // 5. Open the new session dialog and start a slow second session that
    //    stays RUNNING long enough to assert the sidebar reaction.
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await session
      .newSessionPromptInput()
      .fill('e2e:message("starting...")\ne2e:delay(15000)\ne2e:message("done")');
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    // 6. Backend must have created a second session.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.length;
        },
        { timeout: 30_000, message: "Waiting for second session to appear" },
      )
      .toBe(2);

    // 7. Regression assertion: sidebar moves the task into the "Running"
    //    bucket while the secondary session is still mid-delay.
    await expect(session.taskInSection(TASK_TITLE, "Running")).toBeVisible({
      timeout: 30_000,
    });

    // 8. After the delay completes, the task should land back in
    //    "Turn Finished" — proving the fix doesn't break the normal
    //    completion path either.
    await expect(
      session.taskInSection(TASK_TITLE, "Turn Finished"),
    ).toBeVisible({ timeout: 45_000 });
  });
});
