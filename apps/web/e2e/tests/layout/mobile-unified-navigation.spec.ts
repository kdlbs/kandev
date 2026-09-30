import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

// @covers AC-UI-MOBILE-MENU-001.1, AC-UI-MOBILE-MENU-001.2, AC-UI-MOBILE-MENU-001.5
test("same menu from listings and page shells", async ({ testPage }) => {
  for (const path of ["/?home=overview", "/tasks", "/threads", "/stats"]) {
    await testPage.goto(path);
    const trigger = testPage.getByTestId("app-nav-trigger");
    await trigger.tap();
    const menu = testPage.getByTestId("app-nav-sheet");
    await expect(menu).toHaveAttribute("data-vaul-drawer-direction", "bottom");
    await expect(menu.getByTestId("mobile-workspace-trigger")).toBeVisible();
    for (const name of ["Home", "Settings"]) {
      await expect(menu.getByRole("link", { name, exact: true })).toBeVisible();
    }
    await expect(menu.getByTestId("mobile-search-toggle")).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage);
    await testPage.keyboard.press("Escape");
    await expect(menu).toBeHidden();
    await expect(trigger).toBeFocused();
  }
});

// @covers AC-UI-MOBILE-MENU-001.4, AC-UI-MOBILE-MENU-001.6
test("separate listing controls preserve search behavior", async ({ testPage }) => {
  await testPage.goto("/tasks");
  const options = testPage.getByTestId("mobile-topbar-page-context");
  await options.tap();
  const sheet = testPage.getByRole("dialog", { name: "View options", exact: true });
  await expect(sheet.getByRole("link", { name: "Settings", exact: true })).toHaveCount(0);
  await testPage.screenshot({ path: test.info().outputPath("view-options-393.png") });
  await sheet.getByTestId("mobile-search-toggle").tap();
  const search = testPage.getByPlaceholder("Search tasks...");
  await expect(search).toBeFocused();
  await search.fill("checkout");
  await options.tap();
  await sheet.getByTestId("mobile-search-toggle").tap();
  await expect(search).toBeHidden();
  await expect(options).toBeFocused();
  await options.tap();
  await sheet.getByTestId("mobile-search-toggle").tap();
  await expect(search).toHaveValue("");
  await testPage.setViewportSize({ width: 767, height: 851 });
  await options.tap();
  await expect(sheet).toBeVisible();
  await assertNoDocumentHorizontalOverflow(testPage);
  await testPage.screenshot({ path: test.info().outputPath("view-options-767.png") });
});

// @covers AC-UI-MOBILE-MENU-002.1, AC-UI-MOBILE-MENU-002.2, AC-UI-MOBILE-MENU-002.3
test("title picker switches tasks without changing hamburger meaning", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const first = await apiClient.seedTask(seedData.workspaceId, "First navigation task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const second = await apiClient.seedTask(seedData.workspaceId, "Second navigation task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await testPage.goto(`/t/${first.task_id}`);
  const title = testPage.getByTestId("mobile-task-picker-trigger");
  await expect(title).toContainText("First navigation task");
  await testPage.screenshot({ path: test.info().outputPath("task-title-393.png") });
  await testPage.getByTestId("app-nav-trigger").tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  await expect(menu.getByRole("link", { name: "Tasks", exact: true })).toHaveCount(0);
  await expect(menu.getByTestId("mobile-navigation-tasks-toggle")).toBeVisible();
  await testPage.keyboard.press("Escape");
  await title.tap();
  const picker = testPage.getByRole("dialog", { name: "Tasks", exact: true });
  await expect(picker).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(title).toBeFocused();
  await title.tap();
  await picker.locator(`[data-task-row-id="${second.task_id}"]`).tap();
  await expect(testPage).toHaveURL(new RegExp(`/t/${second.task_id}`));
  await expect(title).toContainText("Second navigation task");
  await assertNoDocumentHorizontalOverflow(testPage);
  await testPage.setViewportSize({ width: 767, height: 851 });
  await assertNoDocumentHorizontalOverflow(testPage);
  await testPage.screenshot({ path: test.info().outputPath("task-title-767.png") });
});

// @covers AC-UI-MOBILE-MENU-001.5, AC-UI-MOBILE-MENU-001.6
test("drawer geometry and focus survive breakpoint changes", async ({ testPage }) => {
  for (const width of [393, 767]) {
    await testPage.setViewportSize({ width, height: 851 });
    await testPage.goto("/stats");
    const trigger = testPage.getByTestId("app-nav-trigger");
    await trigger.tap();
    const menu = testPage.getByTestId("app-nav-sheet");
    await expect(menu).toHaveAttribute("data-vaul-drawer-direction", "bottom");
    await menu.evaluate(async (element) => {
      await Promise.all(
        element.getAnimations({ subtree: true }).map((animation) => animation.finished),
      );
    });
    const box = await menu.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.y).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    expect(box!.y + box!.height).toBeLessThanOrEqual(851);
    const target = await trigger.boundingBox();
    expect(target!.height).toBeGreaterThanOrEqual(44);
    expect(target!.width).toBeGreaterThanOrEqual(44);
    await testPage.screenshot({ path: test.info().outputPath(`navigation-${width}.png`) });
    await testPage.keyboard.press("Escape");
    await expect(trigger).toBeFocused();
    await assertNoDocumentHorizontalOverflow(testPage);
  }
  for (const width of [768, 820]) {
    await testPage.setViewportSize({ width, height: 851 });
    await expect(testPage.getByTestId("app-nav-trigger")).toBeHidden();
    await expect(testPage.getByTestId("app-nav-sheet")).toBeHidden();
  }
});

// @covers AC-UI-MOBILE-MENU-001.6
test("a task-picker creation draft survives phone rotation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.seedTask(seedData.workspaceId, "Task with a rotation draft", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await testPage.goto(`/t/${task.task_id}`);
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  await testPage
    .getByRole("dialog", { name: "Tasks", exact: true })
    .getByRole("button", { name: "New", exact: true })
    .tap();
  const title = testPage.getByTestId("create-task-dialog").getByTestId("task-title-input");
  await title.fill("Draft survives task rotation");
  await testPage.setViewportSize({ width: 851, height: 393 });
  await expect(title).toHaveValue("Draft survives task rotation");
});
