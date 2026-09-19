import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

// @covers AC-UI-MOBILE-MENU-004.1, 004.2, 004.3
test("shared navigation embeds the complete collapsible task sidebar", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.seedTask(seedData.workspaceId, "Embedded navigation task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  for (let index = 0; index < 5; index++) {
    await apiClient.seedTask(seedData.workspaceId, `Another sidebar task ${index}`, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
  }
  await testPage.goto("/stats");
  await testPage.getByTestId("app-nav-trigger").tap();
  const menu = testPage.getByTestId("app-nav-sheet");
  const toggle = menu.getByTestId("mobile-navigation-tasks-toggle");
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await expect(menu.getByRole("button", { name: "Task views", exact: true })).toHaveCount(0);
  await expect(menu.getByTestId("mobile-pinned-tasks")).toHaveCount(0);
  await expect(menu.getByText("Embedded navigation task", { exact: true })).toBeVisible();
  await toggle.tap();
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await expect(menu.getByText("Embedded navigation task", { exact: true })).toBeHidden();
  await expect(menu.getByRole("link", { name: "Settings", exact: true })).toBeVisible();
  await testPage.keyboard.press("Escape");
  await testPage.getByTestId("app-nav-trigger").tap();
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await toggle.tap();
  await expect(menu.locator("[data-task-row-id]")).toHaveCount(6);
  expect(
    await menu
      .getByTestId("mobile-task-switcher-list")
      .evaluate((el) => getComputedStyle(el).overflowY),
  ).toBe("visible");
  await assertNoDocumentHorizontalOverflow(testPage);
  await testPage.screenshot({ path: test.info().outputPath("embedded-tasks-393.png") });
  await testPage.setViewportSize({ width: 767, height: 851 });
  await assertNoDocumentHorizontalOverflow(testPage);
  await testPage.screenshot({ path: test.info().outputPath("embedded-tasks-767.png") });
  await menu.getByText("Embedded navigation task", { exact: true }).tap();
  await expect(testPage).toHaveURL(new RegExp(`/t/${task.task_id}`));
  await testPage.goBack();
  await expect(testPage).toHaveURL(/\/stats$/);
});

// @covers AC-UI-MOBILE-MENU-005.1, 005.2, 005.3
for (const route of ["/", "/tasks", "/threads"]) {
  test(`listing title opens all view options on ${route}`, async ({ testPage }) => {
    await testPage.goto(route);
    const trigger = testPage.getByTestId("mobile-topbar-page-context");
    await expect(trigger).toBeVisible();
    await expect(testPage.getByTestId("mobile-view-options-trigger")).toHaveCount(0);
    await trigger.tap();
    const options = testPage.getByRole("dialog", { name: "View options", exact: true });
    await expect(options).toBeVisible();
    await expect(options.getByRole("radio", { name: "Kanban", exact: true })).toBeVisible();
    await expect(options.getByRole("radio", { name: "Threads", exact: true })).toBeVisible();
    await expect(options.getByRole("radio", { name: "List", exact: true })).toBeVisible();
    if (route === "/threads")
      await expect(options.getByTestId("threads-mobile-view-list")).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(trigger).toBeFocused();
  });
}

// @covers AC-UI-MOBILE-MENU-004.3
for (const status of [503, 403]) {
  test(`embedded Tasks recovers from a ${status} workspace read`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.seedTask(seedData.workspaceId, "Workspace-scoped task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    let unavailable = true;
    await testPage.route("**/api/v1/workflows?*", async (route) => {
      if (unavailable) await route.fulfill({ status, json: { error: "unavailable" } });
      else await route.continue();
    });
    await testPage.goto("/stats");
    await testPage.getByTestId("app-nav-trigger").tap();
    const menu = testPage.getByTestId("app-nav-sheet");
    await expect(menu.getByTestId("sidebar-task-load-error")).toBeVisible();
    await expect(menu.getByRole("link", { name: "Settings", exact: true })).toBeVisible();
    if (status === 403)
      await expect(menu.getByText("Workspace-scoped task", { exact: true })).toHaveCount(0);
    unavailable = false;
    await menu.getByRole("button", { name: "Retry", exact: true }).tap();
    await expect(menu.getByTestId("sidebar-task-load-error")).toHaveCount(0);
    await expect(menu.getByText("Workspace-scoped task", { exact: true })).toBeVisible();
  });
}
