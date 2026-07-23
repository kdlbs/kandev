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
test.describe("sidebar scrolling", () => {
  test("fades overflowing tasks and reveals the scrollbar on hover", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const taskCount = 25;
    const taskIds: string[] = [];
    for (let index = 0; index < taskCount; index++) {
      const task = await apiClient.createTask(
        seedData.workspaceId,
        `Overflow Task ${String(index).padStart(2, "0")}`,
        {
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      taskIds.push(task.id);
    }

    await testPage.goto(`/t/${taskIds.at(-1)!}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const scrollContainer = testPage.getByTestId("task-sidebar-scroll");
    await expect(scrollContainer).toBeVisible();
    await expect(session.sidebar.getByTestId("sidebar-task-item")).toHaveCount(taskCount, {
      timeout: 10_000,
    });

    const overlayGeometry = await scrollContainer.evaluate((element) => {
      // Headless Chromium uses platform overlay scrollbars, while desktop
      // WebViews can reserve a native gutter. Force that platform mode so the
      // regression is deterministic in CI.
      element.style.scrollbarGutter = "stable";
      const row = element.querySelector<HTMLElement>("[data-testid='sidebar-task-item']");
      if (!row) throw new Error("Expected a rendered sidebar task row");
      const containerRect = element.getBoundingClientRect();
      const rowRect = row.getBoundingClientRect();
      return {
        rowRightGap: containerRect.right - rowRect.right,
      };
    });
    expect(overlayGeometry.rowRightGap).toBeLessThanOrEqual(1);

    await expect(scrollContainer).toHaveAttribute("data-can-scroll-down", "true");
    const overlayScrollbar = session.sidebar.locator(
      "[data-slot='scroll-area-scrollbar'][data-orientation='vertical']",
    );
    await expect(overlayScrollbar).toBeAttached();
    const restingStyles = await scrollContainer.evaluate((element) => {
      const styles = getComputedStyle(element);
      return {
        maskImage: styles.maskImage,
      };
    });
    expect(restingStyles.maskImage).toContain("linear-gradient");
    await expect(overlayScrollbar).toHaveCSS("opacity", "0");

    await scrollContainer.hover();
    const hoverStyles = await scrollContainer.evaluate((element) => {
      const styles = getComputedStyle(element);
      return {
        maskImage: styles.maskImage,
      };
    });
    expect(hoverStyles.maskImage).toBe("none");
    await expect(overlayScrollbar).toHaveCSS("opacity", "1");

    await scrollContainer.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect(scrollContainer).toHaveAttribute("data-can-scroll-down", "false");
  });

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
