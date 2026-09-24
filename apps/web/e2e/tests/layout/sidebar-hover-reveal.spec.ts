import { test, expect } from "../../fixtures/test-base";
import type { Locator } from "@playwright/test";

async function waitForSidebarReveal(sidebar: Locator) {
  await expect(sidebar).toHaveAttribute("data-hover-revealed", "true", { timeout: 15_000 });
  await expect(sidebar).toHaveCSS("width", "320px", { timeout: 15_000 });
}

// @covers AC-UI-SIDEBAR-HOVER-001.1, AC-UI-SIDEBAR-HOVER-001.2
test("collapsed sidebar reveals after dwell without moving page content", async ({
  testPage,
}, testInfo) => {
  await testPage.goto("/");
  const sidebar = testPage.getByTestId("app-sidebar");
  await sidebar.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
  await testPage.mouse.move(600, 400);
  await expect(sidebar).toHaveCSS("width", "56px");
  const pageBefore = await testPage
    .getByTestId("app-shell")
    .locator(":scope > div > main")
    .boundingBox();
  await sidebar.hover({ position: { x: 20, y: 200 } });
  await waitForSidebarReveal(sidebar);
  await expect(testPage.getByTestId("app-sidebar-layout")).toHaveCSS("width", "56px");
  await expect(sidebar).toHaveAttribute("data-collapsed", "true");
  const pageAfter = await testPage
    .getByTestId("app-shell")
    .locator(":scope > div > main")
    .boundingBox();
  expect(pageAfter).toEqual(pageBefore);
  await testPage.screenshot({ path: testInfo.outputPath("sidebar-hover.png") });
  await testPage.mouse.move(600, 400);
  await expect(sidebar).toHaveCSS("width", "56px");
});

// @covers AC-UI-SIDEBAR-HOVER-001.3, AC-UI-SIDEBAR-HOVER-001.4
test("reveal supports the workspace menu, Escape, and permanent expansion", async ({
  testPage,
}) => {
  await testPage.goto("/");
  const sidebar = testPage.getByTestId("app-sidebar");
  await sidebar.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
  await testPage.mouse.move(600, 400);
  await expect(sidebar).toHaveCSS("width", "56px");
  await sidebar.hover({ position: { x: 20, y: 200 } });
  await waitForSidebarReveal(sidebar);
  await expect(sidebar.getByTestId("sidebar-workspace-trigger")).toBeVisible();
  await sidebar.getByTestId("sidebar-workspace-trigger").click();
  const menu = testPage.getByRole("menu");
  await expect(menu).toBeVisible();
  await menu.hover();
  await expect(sidebar).toHaveCSS("width", "320px");
  await testPage.keyboard.press("Escape");
  await expect(menu).toBeHidden();
  await expect(sidebar).toHaveCSS("width", "320px");
  await testPage.keyboard.press("Escape");
  await expect(sidebar).toHaveCSS("width", "56px");
  await expect(sidebar.getByRole("button", { name: "Expand sidebar", exact: true })).toBeFocused();
  await testPage.mouse.move(600, 400);
  await sidebar.hover({ position: { x: 20, y: 200 } });
  await expect(sidebar.getByTestId("sidebar-workspace-trigger")).toBeVisible();
  await sidebar.getByRole("button", { name: "Expand sidebar", exact: true }).click();
  await testPage.mouse.move(600, 400);
  await expect(sidebar).toHaveAttribute("data-collapsed", "false");
  await expect(testPage.getByTestId("app-sidebar-layout")).toHaveCSS("width", "320px");
  await testPage.reload();
  await expect(sidebar).toHaveAttribute("data-collapsed", "false");
});

// @covers AC-UI-SIDEBAR-HOVER-001.1, AC-UI-SIDEBAR-HOVER-001.5
test("short hover cancels and crossing the phone breakpoint closes the reveal", async ({
  testPage,
}) => {
  await testPage.goto("/");
  const sidebar = testPage.getByTestId("app-sidebar");
  await sidebar.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
  await testPage.mouse.move(600, 400);
  await expect(sidebar).toHaveCSS("width", "56px");
  await testPage.clock.install();
  await testPage.clock.pauseAt(await testPage.evaluate(() => Date.now() + 100));
  await sidebar.hover({ position: { x: 20, y: 200 } });
  await testPage.clock.runFor(499);
  await expect(sidebar).toHaveCSS("width", "56px");
  await testPage.mouse.move(600, 400);
  await testPage.clock.runFor(1000);
  await expect(sidebar).toHaveCSS("width", "56px");
  await testPage.clock.resume();
  await testPage.setViewportSize({ width: 768, height: 900 });
  await sidebar.hover({ position: { x: 20, y: 200 } });
  await waitForSidebarReveal(sidebar);
  await testPage.setViewportSize({ width: 767, height: 900 });
  await expect(sidebar).toBeHidden();
  await testPage.mouse.move(600, 400);
  await testPage.setViewportSize({ width: 768, height: 900 });
  await expect(sidebar).toHaveCSS("width", "56px");
  await expect(sidebar).toHaveAttribute("data-collapsed", "true");
});

// @covers AC-UI-SIDEBAR-HOVER-001.3
test("revealed tasks support context menus and navigation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Hover navigation task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await testPage.goto("/");
  const sidebar = testPage.getByTestId("app-sidebar");
  await sidebar.getByRole("button", { name: "Collapse sidebar", exact: true }).click();
  await testPage.mouse.move(600, 400);
  await expect(sidebar).toHaveCSS("width", "56px");
  await sidebar.hover({ position: { x: 20, y: 200 } });
  await waitForSidebarReveal(sidebar);
  const row = sidebar.getByText("Hover navigation task", { exact: true });
  await expect(row).toBeVisible();
  await row.click({ button: "right" });
  const menu = testPage.getByRole("menu");
  await expect(menu).toBeVisible();
  await menu.hover();
  await expect(row).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(menu).toBeHidden();
  await row.click();
  await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));
  await expect(sidebar).toHaveCSS("width", "56px");
});
