import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

// Regression: when a previously WAITING_FOR_INPUT session is auto-resumed
// after a backend restart, the task must NOT transiently jump out of the
// "Turn Finished" (review) bucket into the "Running" (in_progress) bucket
// in the sidebar. The bug caused the task to briefly transition through
// session states STARTING -> RUNNING -> WAITING_FOR_INPUT, which moved it
// to the top of the sidebar before settling back.

test.describe("Session resume — sidebar order is stable", () => {
  test.describe.configure({ retries: 1 });

  test("task does not jump buckets during silent resume after backend restart", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // 1. Create two tasks. Task A is the one we will resume; task B exists so
    //    that any reordering is observable as A moving above B.
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Resume A", seedData.agentProfileId, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    // 2. Open task A and wait for it to reach WAITING_FOR_INPUT (Turn Finished).
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const cardA = kanban.taskCardByTitle("Resume A");
    await expect(cardA).toBeVisible({ timeout: 15_000 });
    await cardA.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.taskInSection("Resume A", "Turn Finished")).toBeVisible({
      timeout: 15_000,
    });

    // 3. Restart the backend and reload the page to trigger auto-resume.
    await backend.restart();
    await testPage.reload();
    await session.waitForLoad();

    // 4. Continuously assert that "Resume A" is NEVER seen in the "Running"
    //    bucket while the resume is in flight. We poll across the full
    //    resume window so any transient STARTING/RUNNING flicker is caught.
    const runningLocator = session.taskInSection("Resume A", "Running");
    const deadline = Date.now() + 30_000;
    while (Date.now() < deadline) {
      const count = await runningLocator.count();
      expect(
        count,
        "Resume A appeared in the Running bucket during silent resume",
      ).toBe(0);
      // Stop polling once the agent is fully idle and the resume is complete.
      if (await session.idleInput().isVisible().catch(() => false)) {
        break;
      }
      await testPage.waitForTimeout(100);
    }

    // 5. Final state: still in Turn Finished after the resume completes.
    await expect(session.idleInput()).toBeVisible({ timeout: 60_000 });
    await expect(session.taskInSection("Resume A", "Turn Finished")).toBeVisible({
      timeout: 15_000,
    });
    await expect(session.taskInSection("Resume A", "Running")).not.toBeVisible({
      timeout: 1_000,
    });
  });
});
