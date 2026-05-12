import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Regression: when clicking another task in the sidebar, the sidebar's
 * scroll position would jump back to the top. Dockview rebuilds panel slots
 * on env switch, which detaches and re-attaches the sidebar's portal element.
 * Browsers reset scrollTop on DOM detach, so the sidebar lost its scroll
 * position. usePortalSlot now snapshots scroll positions inside each portal
 * via a capturing scroll listener and restores them after every reattach.
 */
test.describe("sidebar scroll preservation across task switch", () => {
  test("clicking another task does not reset the sidebar scroll position", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // Seed enough tasks so the sidebar list overflows its viewport. Titles
    // are zero-padded so sort order matches creation order.
    const TASK_COUNT = 25;
    const created: { id: string; title: string }[] = [];
    for (let i = 0; i < TASK_COUNT; i++) {
      const title = `Scroll Task ${String(i).padStart(2, "0")}`;
      const task = await apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });
      created.push({ id: task.id, title });
    }

    // Navigate to the most recently created task (renders at the top of the
    // sidebar since tasks sort by createdAt desc within a state bucket).
    const navTask = created[created.length - 1];
    await testPage.goto(`/t/${navTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    // Wait for the full task list to render in the sidebar.
    await expect(session.sidebar.getByTestId("sidebar-task-item")).toHaveCount(TASK_COUNT, {
      timeout: 10_000,
    });

    const scrollContainer = testPage.getByTestId("task-sidebar-scroll");
    await expect(scrollContainer).toBeVisible();

    // Sanity: list overflows the scroll container.
    const dimensions = await scrollContainer.evaluate((el) => ({
      clientHeight: el.clientHeight,
      scrollHeight: el.scrollHeight,
    }));
    expect(dimensions.scrollHeight).toBeGreaterThan(dimensions.clientHeight);

    // Scroll to the bottom of the sidebar.
    await scrollContainer.evaluate((el) => {
      el.scrollTop = el.scrollHeight - el.clientHeight;
    });
    const scrollBefore = await scrollContainer.evaluate((el) => el.scrollTop);
    expect(scrollBefore).toBeGreaterThan(0);

    // Pick a task that is currently visible near the bottom (the oldest one).
    // The first-created task appears last in the list because of desc sort.
    const bottomTask = created[0];
    const bottomRow = session.sidebarTaskItem(bottomTask.title).first();
    await expect(bottomRow).toBeVisible();
    await expect(bottomRow).toBeInViewport();

    await bottomRow.click();

    // URL switches to the new task once selection settles.
    await expect.poll(() => testPage.url(), { timeout: 10_000 }).toContain(bottomTask.id);

    // Sidebar should still be scrolled near the previous position. Without
    // the fix, scrollTop would snap back to 0 after the dockview slot is
    // re-created. Allow a small tolerance for any minor adjustments.
    await expect
      .poll(() => scrollContainer.evaluate((el) => el.scrollTop), { timeout: 5_000 })
      .toBeGreaterThan(scrollBefore - 50);
  });
});
