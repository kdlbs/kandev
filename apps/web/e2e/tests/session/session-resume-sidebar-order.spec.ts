import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

// Regression: the sidebar statePriority must sort "Turn Finished" (review)
// tasks ABOVE "Running" (in_progress) tasks. Before this fix the priority
// was inverted, which caused a task whose session transiently cycled
// through RUNNING during auto-resume (after a backend restart) to jump to
// the top of the sidebar and displace other turn-finished tasks.
//
// This test asserts the sort order deterministically by seeding two tasks
// with explicit states via the API: one REVIEW, one IN_PROGRESS. The
// review task must render above the in_progress task in the session
// sidebar regardless of createdAt order.

test.describe("Session sidebar — sort order", () => {
  test("review (turn finished) tasks sort above in_progress (running) tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // Create an anchor task with an agent so we can open a session page
    // and view the sidebar in its normal SSR-hydrated context.
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Anchor Session",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Create the two ordering fixtures. Create the REVIEW one FIRST so it
    // has an older createdAt — this rules out createdAt-DESC as the reason
    // for any correct ordering we observe. The only way the newer
    // IN_PROGRESS task ends up BELOW the older REVIEW task is the state
    // bucket priority being review < in_progress.
    const review = await apiClient.createTask(seedData.workspaceId, "Older Review Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.updateTaskState(review.id, "REVIEW");

    const inProgress = await apiClient.createTask(seedData.workspaceId, "Newer Running Fixture", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.updateTaskState(inProgress.id, "IN_PROGRESS");

    // Open the anchor session so the sidebar is visible.
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const anchorCard = kanban.taskCardByTitle("Anchor Session");
    await expect(anchorCard).toBeVisible({ timeout: 15_000 });
    await anchorCard.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for both fixture tasks to appear in the sidebar.
    await expect(session.taskInSidebar("Older Review Fixture")).toBeVisible({ timeout: 15_000 });
    await expect(session.taskInSidebar("Newer Running Fixture")).toBeVisible({ timeout: 15_000 });

    // Resolve DOM order of the two fixture items in the sidebar.
    const sidebarItems = session.sidebar.getByTestId("sidebar-task-item");
    const indexOf = async (title: string): Promise<number> => {
      const count = await sidebarItems.count();
      for (let i = 0; i < count; i++) {
        const text = await sidebarItems.nth(i).innerText();
        if (text.includes(title)) return i;
      }
      return -1;
    };

    const reviewIdx = await indexOf("Older Review Fixture");
    const runningIdx = await indexOf("Newer Running Fixture");

    expect(reviewIdx).not.toBe(-1);
    expect(runningIdx).not.toBe(-1);
    expect(
      reviewIdx,
      `review task must sort above in_progress task (got reviewIdx=${reviewIdx}, runningIdx=${runningIdx})`,
    ).toBeLessThan(runningIdx);

    // Persistence check — reload the page and verify the order holds via
    // SSR hydration, not just client-side state.
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.taskInSidebar("Older Review Fixture")).toBeVisible({ timeout: 15_000 });
    await expect(session.taskInSidebar("Newer Running Fixture")).toBeVisible({ timeout: 15_000 });

    const reviewIdxAfter = await indexOf("Older Review Fixture");
    const runningIdxAfter = await indexOf("Newer Running Fixture");
    expect(reviewIdxAfter).toBeLessThan(runningIdxAfter);
  });
});
