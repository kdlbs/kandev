import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { dwell } from "../../helpers/causal-waits";

const OVERLAY_SCROLLBAR_SELECTOR =
  "[data-slot='scroll-area-scrollbar'][data-orientation='vertical']";

/**
 * Regression: when clicking another task in the sidebar, the sidebar's
 * scroll position would jump back to the top. Dockview rebuilds panel slots
 * on env switch, which detaches and re-attaches the sidebar's portal element.
 * Browsers reset scrollTop on DOM detach, so the sidebar lost its scroll
 * position. usePortalSlot now snapshots scroll positions inside each portal
 * via a capturing scroll listener and restores them after every reattach.
 */
test.describe("sidebar scrolling", () => {
  test("keeps the empty list transparent and uses the full sidebar width", async ({ testPage }) => {
    await testPage.goto("/");

    const tasksToggle = testPage.getByRole("button", { name: "Tasks", exact: true });
    if ((await tasksToggle.getAttribute("aria-expanded")) !== "true") {
      await tasksToggle.click();
    }

    const appSidebar = testPage.getByTestId("app-sidebar");
    const taskSidebar = testPage.getByTestId("task-sidebar");
    const scrollRoot = taskSidebar.locator("[data-slot='scroll-area']");
    const emptyMessage = taskSidebar.locator("[data-slot='task-switcher-empty-state']");
    await expect(emptyMessage).toHaveText("No tasks yet.");

    await expect
      .poll(() => scrollRoot.evaluate((element) => getComputedStyle(element).backgroundColor))
      .toBe("rgba(0, 0, 0, 0)");

    const [navigationBox, taskSidebarBox, tasksTitleBox, emptyMessageBox, emptyPaddingLeft] =
      await Promise.all([
        appSidebar.locator("nav").boundingBox(),
        taskSidebar.boundingBox(),
        tasksToggle.locator("span").first().boundingBox(),
        emptyMessage.boundingBox(),
        emptyMessage.evaluate((element) => parseFloat(getComputedStyle(element).paddingLeft)),
      ]);
    expect(navigationBox).not.toBeNull();
    expect(taskSidebarBox).not.toBeNull();
    expect(tasksTitleBox).not.toBeNull();
    expect(emptyMessageBox).not.toBeNull();
    expect(taskSidebarBox!.x).toBeCloseTo(navigationBox!.x, 0);
    expect(taskSidebarBox!.x + taskSidebarBox!.width).toBeCloseTo(
      navigationBox!.x + navigationBox!.width,
      0,
    );
    expect(Math.abs(emptyMessageBox!.x + emptyPaddingLeft - tasksTitleBox!.x)).toBeLessThanOrEqual(
      0.5,
    );
  });

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
    const overlayScrollbar = session.sidebar.locator(OVERLAY_SCROLLBAR_SELECTOR);
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

  test("does not mount an overlay scrollbar when the task list fits", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Short Task List", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const scrollContainer = testPage.getByTestId("task-sidebar-scroll");
    await expect(scrollContainer).toHaveAttribute("data-can-scroll-down", "false");
    await expect(session.sidebar.locator(OVERLAY_SCROLLBAR_SELECTOR)).toHaveCount(0);
  });

  test("keeps long task titles and hover actions inside the sidebar", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const longTitle = "Long sidebar title scrolls on hover with room for actions";
    const task = await apiClient.createTask(seedData.workspaceId, longTitle, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const scrollContainer = session.sidebar.getByTestId("task-sidebar-scroll");
    const taskRow = session.sidebar.getByTestId("sidebar-task-item").filter({ hasText: longTitle });
    const titleText = taskRow.getByText(longTitle, { exact: true });
    const titleViewport = titleText.locator("..");
    const actions = taskRow.getByRole("button", { name: "Task actions" });

    await expect(taskRow).toBeVisible();
    const overflow = await titleViewport.evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
    }));
    expect(overflow.scrollWidth).toBeGreaterThan(overflow.clientWidth);

    await titleViewport.hover();
    await expect
      .poll(() => titleText.evaluate((element) => element.style.transform))
      .toMatch(/^translateX\(-/);
    await expect(actions.locator("..")).toHaveCSS("opacity", "1");
    await expect(actions).toBeInViewport();

    const [containerBox, rowBox, titleBox, actionBox] = await Promise.all([
      scrollContainer.boundingBox(),
      taskRow.boundingBox(),
      titleViewport.boundingBox(),
      actions.boundingBox(),
    ]);
    if (!containerBox || !rowBox || !titleBox || !actionBox) {
      throw new Error("Long-title sidebar row has no layout box");
    }
    expect(rowBox.x + rowBox.width).toBeLessThanOrEqual(containerBox.x + containerBox.width + 1);
    expect(rowBox.x + rowBox.width - (actionBox.x + actionBox.width)).toBeGreaterThanOrEqual(11);
    expect(actionBox.x - (titleBox.x + titleBox.width)).toBeGreaterThanOrEqual(7);
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

  test("reveals a command-selected task", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(60_000);

    const taskCount = 25;
    const created: { id: string; title: string }[] = [];
    for (let index = 0; index < taskCount; index++) {
      const title = `Command Reveal Task ${String(index).padStart(2, "0")}`;
      const task = await apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });
      created.push({ id: task.id, title });
    }

    const initialTask = created.at(-1)!;
    await testPage.goto(`/t/${initialTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const scrollContainer = testPage.getByTestId("task-sidebar-scroll");
    await expect(scrollContainer).toBeVisible();
    await expect(session.sidebar.getByTestId("sidebar-task-item")).toHaveCount(taskCount, {
      timeout: 10_000,
    });
    await scrollContainer.evaluate((element) => {
      element.scrollTop = 0;
    });

    const dimensions = await scrollContainer.evaluate((element) => ({
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
    }));
    expect(dimensions.scrollHeight).toBeGreaterThan(dimensions.clientHeight);

    const offscreenTitle = await scrollContainer.evaluate(
      (element, taskTitles) => {
        const containerRect = element.getBoundingClientRect();
        const rows = element.querySelectorAll<HTMLElement>("[data-testid='sidebar-task-item']");
        for (const row of rows) {
          const rowRect = row.getBoundingClientRect();
          const isOutside =
            rowRect.bottom <= containerRect.top + 1 || rowRect.top >= containerRect.bottom - 1;
          if (isOutside) {
            const title = taskTitles.find((candidate) => row.textContent?.includes(candidate));
            if (title) return title;
          }
        }
        return null;
      },
      created.map(({ title }) => title),
    );
    if (!offscreenTitle) throw new Error("Expected a rendered task row outside the viewport");
    const targetTask = created.find(({ title }) => title === offscreenTitle)!;
    const targetRow = session.sidebarTaskItem(targetTask.title);
    await expect(targetRow).toBeVisible();
    const before = await Promise.all([scrollContainer.boundingBox(), targetRow.boundingBox()]);
    if (!before[0] || !before[1]) throw new Error("Command-selected target has no layout box");
    expect(
      before[1].y + before[1].height <= before[0].y + 1 ||
        before[1].y >= before[0].y + before[0].height - 1,
      "target task should start outside the task-list viewport",
    ).toBe(true);

    const documentScrollBefore = await testPage.evaluate(() => ({ scrollX, scrollY }));
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+k`);
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await dialog.getByRole("combobox").fill(targetTask.title);
    const option = dialog.getByRole("option").filter({ hasText: targetTask.title });
    await expect(option).toBeVisible({ timeout: 10_000 });
    await option.click();

    await expect(testPage).toHaveURL(new RegExp(`/t/${targetTask.id}$`));
    await expect(session.activeSidebarTaskItem(targetTask.title).first()).toHaveAttribute(
      "aria-current",
      "true",
    );
    await expect(targetRow).toHaveClass(/task-sidebar-row-reveal/, { timeout: 1_000 });
    await expect
      .poll(
        async () => {
          const [containerBox, rowBox] = await Promise.all([
            scrollContainer.boundingBox(),
            targetRow.boundingBox(),
          ]);
          if (!containerBox || !rowBox) return false;
          return (
            rowBox.y >= containerBox.y - 1 &&
            rowBox.y + rowBox.height <= containerBox.y + containerBox.height + 1
          );
        },
        { timeout: 10_000 },
      )
      .toBe(true);

    await expect
      .poll(() => testPage.evaluate(() => ({ scrollX, scrollY })))
      .toEqual(documentScrollBefore);
  });

  test("reveals a command-selected task after a delayed settings navigation blocker", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const initialSettings = await apiClient.getUserSettings();
    const initialLayout =
      initialSettings.settings.changes_panel_layout === "tree" ? "tree" : "flat";
    const nextLayout = initialLayout === "tree" ? "Flat list" : "Tree";
    const taskCount = 25;
    const created: { id: string; title: string }[] = [];

    try {
      for (let index = 0; index < taskCount; index++) {
        const title = `Blocked Command Reveal Task ${String(index).padStart(2, "0")}`;
        const task = await apiClient.createTask(seedData.workspaceId, title, {
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        });
        created.push({ id: task.id, title });
      }

      const targetTask = created[0];
      await testPage.goto("/settings/general/appearance");
      await expect(
        testPage.getByRole("heading", { level: 2, name: "Appearance", exact: true }),
      ).toBeVisible();

      await testPage.getByTestId("changes-panel-layout-select").click();
      await testPage.getByRole("option", { name: nextLayout }).click();
      await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();

      const modifier = process.platform === "darwin" ? "Meta" : "Control";
      await testPage.keyboard.press(`${modifier}+k`);
      const commandDialog = testPage.getByRole("dialog");
      await expect(commandDialog).toBeVisible({ timeout: 5_000 });
      await commandDialog.getByRole("combobox").fill(targetTask.title);
      const option = commandDialog.getByRole("option").filter({ hasText: targetTask.title });
      await expect(option).toBeVisible({ timeout: 10_000 });
      await option.click();

      const navigationDialog = testPage.getByRole("alertdialog", {
        name: "Save changes before leaving?",
      });
      await expect(navigationDialog).toBeVisible();

      await dwell(
        testPage,
        1_500,
        "product-timer",
        "the regression under test is a frame-count reveal budget expiring while the sidebar is absent from the DOM; the budget's expiry is the event and nothing renders to signal it",
      );
      await navigationDialog.getByRole("button", { name: "Discard and leave" }).click();

      await expect(testPage).toHaveURL(new RegExp(`/t/${targetTask.id}$`));
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      const scrollContainer = testPage.getByTestId("task-sidebar-scroll");
      const targetRow = session.sidebarTaskItem(targetTask.title);
      await expect(scrollContainer).toBeVisible();
      await expect(session.sidebar.getByTestId("sidebar-task-item")).toHaveCount(taskCount, {
        timeout: 10_000,
      });
      await expect(session.activeSidebarTaskItem(targetTask.title).first()).toHaveAttribute(
        "aria-current",
        "true",
      );

      await expect
        .poll(
          async () => {
            const [containerBox, rowBox] = await Promise.all([
              scrollContainer.boundingBox(),
              targetRow.boundingBox(),
            ]);
            if (!containerBox || !rowBox) return false;
            return (
              rowBox.y >= containerBox.y - 1 &&
              rowBox.y + rowBox.height <= containerBox.y + containerBox.height + 1
            );
          },
          { timeout: 10_000 },
        )
        .toBe(true);
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        changes_panel_layout: initialLayout,
      });
    }
  });

  test("reveals a command-selected task that starts above the sidebar viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const taskCount = 25;
    const created: { id: string; title: string }[] = [];
    for (let index = 0; index < taskCount; index++) {
      const title = `Command Reveal Above Task ${String(index).padStart(2, "0")}`;
      const task = await apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      });
      created.push({ id: task.id, title });
    }

    const initialTask = created.at(-1)!;
    await testPage.goto(`/t/${initialTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const scrollContainer = testPage.getByTestId("task-sidebar-scroll");
    await expect(scrollContainer).toBeVisible();
    await expect(session.sidebar.getByTestId("sidebar-task-item")).toHaveCount(taskCount, {
      timeout: 10_000,
    });
    await scrollContainer.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect(scrollContainer).toHaveAttribute("data-can-scroll-down", "false");

    const aboveTitle = await scrollContainer.evaluate(
      (element, taskTitles) => {
        const containerRect = element.getBoundingClientRect();
        const rows = element.querySelectorAll<HTMLElement>("[data-testid='sidebar-task-item']");
        for (const row of rows) {
          const rowRect = row.getBoundingClientRect();
          if (rowRect.bottom <= containerRect.top + 1) {
            const title = taskTitles.find((candidate) => row.textContent?.includes(candidate));
            if (title) return title;
          }
        }
        return null;
      },
      created.map(({ title }) => title),
    );
    if (!aboveTitle) throw new Error("Expected a rendered task row above the viewport");
    const targetTask = created.find(({ title }) => title === aboveTitle)!;
    const targetRow = session.sidebarTaskItem(targetTask.title);
    const before = await Promise.all([scrollContainer.boundingBox(), targetRow.boundingBox()]);
    if (!before[0] || !before[1]) throw new Error("Command-selected target has no layout box");
    expect(before[1].y + before[1].height).toBeLessThanOrEqual(before[0].y + 1);

    const documentScrollBefore = await testPage.evaluate(() => ({ scrollX, scrollY }));
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+k`);
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await dialog.getByRole("combobox").fill(targetTask.title);
    const option = dialog.getByRole("option").filter({ hasText: targetTask.title });
    await expect(option).toBeVisible({ timeout: 10_000 });
    await option.click();

    await expect(testPage).toHaveURL(new RegExp(`/t/${targetTask.id}$`));
    await expect(session.activeSidebarTaskItem(targetTask.title).first()).toHaveAttribute(
      "aria-current",
      "true",
    );
    await expect
      .poll(
        async () => {
          const [containerBox, rowBox] = await Promise.all([
            scrollContainer.boundingBox(),
            targetRow.boundingBox(),
          ]);
          if (!containerBox || !rowBox) return false;
          return (
            rowBox.y >= containerBox.y - 1 &&
            rowBox.y + rowBox.height <= containerBox.y + containerBox.height + 1
          );
        },
        { timeout: 10_000 },
      )
      .toBe(true);

    await expect
      .poll(() => testPage.evaluate(() => ({ scrollX, scrollY })))
      .toEqual(documentScrollBefore);
  });
});
