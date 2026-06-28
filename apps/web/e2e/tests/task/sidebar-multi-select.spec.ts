/**
 * E2E: cmd/ctrl-click and shift-click multi-select in the task sidebar, plus the
 * selection-aware right-click context menu (bulk archive / single fallback).
 */
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { SidebarTasksPage } from "../../pages/sidebar-tasks-page";

type Created = { id: string; title: string };

async function seedTasks(
  apiClient: { createTask: (ws: string, title: string, opts: object) => Promise<{ id: string }> },
  seedData: { workspaceId: string; workflowId: string; startStepId: string },
  titles: string[],
): Promise<Created[]> {
  const out: Created[] = [];
  for (const title of titles) {
    const t = await apiClient.createTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    out.push({ id: t.id, title });
  }
  return out;
}

/** Rendered top-to-bottom order of task ids in the sidebar. */
async function sidebarOrder(page: Page): Promise<string[]> {
  return page
    .getByTestId("task-sidebar")
    .first()
    .locator("[data-testid='sortable-task-block']")
    .evaluateAll((els: Element[]) => els.map((e) => e.getAttribute("data-task-id") ?? ""));
}

async function openSidebarOn(page: Page, taskId: string): Promise<SidebarTasksPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(session.sidebar).toBeVisible({ timeout: 10_000 });
  return new SidebarTasksPage(page);
}

test.describe("Sidebar cmd/shift multi-select", () => {
  test("cmd-click selects a row without navigating", async ({ testPage, apiClient, seedData }) => {
    const [nav, target] = await seedTasks(apiClient, seedData, ["SB Nav", "SB Target"]);
    const sidebar = await openSidebarOn(testPage, nav.id);

    await sidebar.cmdClick(target.id);

    await sidebar.expectSelected(target.id, true);
    // URL must not change — cmd-click selects, it doesn't open the task.
    await expect(testPage).toHaveURL(new RegExp(`/t/${nav.id}`));
  });

  test("cmd-click toggles multiple rows in and out", async ({ testPage, apiClient, seedData }) => {
    const [nav, a, b] = await seedTasks(apiClient, seedData, ["SB MultiNav", "SB A", "SB B"]);
    const sidebar = await openSidebarOn(testPage, nav.id);

    await sidebar.cmdClick(a.id);
    await sidebar.cmdClick(b.id);
    expect(await sidebar.selectedCount()).toBe(2);

    await sidebar.cmdClick(a.id);
    await sidebar.expectSelected(a.id, false);
    await sidebar.expectSelected(b.id, true);
    expect(await sidebar.selectedCount()).toBe(1);
  });

  test("shift-click selects a contiguous range of visible rows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [nav] = await seedTasks(apiClient, seedData, [
      "SB Range Nav",
      "SB Range 1",
      "SB Range 2",
      "SB Range 3",
    ]);
    const sidebar = await openSidebarOn(testPage, nav.id);
    await expect(sidebar.rows().first()).toBeVisible({ timeout: 10_000 });

    const order = await sidebarOrder(testPage);
    expect(order.length).toBeGreaterThanOrEqual(4);

    await sidebar.cmdClick(order[0]);
    await sidebar.shiftClick(order[2]);

    await sidebar.expectSelected(order[0], true);
    await sidebar.expectSelected(order[1], true);
    await sidebar.expectSelected(order[2], true);
    await sidebar.expectSelected(order[3], false);
  });

  test("plain click navigates when nothing is selected", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [nav, target] = await seedTasks(apiClient, seedData, ["SB PlainNav", "SB PlainTarget"]);
    const sidebar = await openSidebarOn(testPage, nav.id);

    await sidebar.plainClick(target.id);
    await expect(testPage).toHaveURL(new RegExp(`/t/${target.id}`));
  });

  test("plain click toggles while a selection is active", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [nav, a, b] = await seedTasks(apiClient, seedData, ["SB PT Nav", "SB PT A", "SB PT B"]);
    const sidebar = await openSidebarOn(testPage, nav.id);

    await sidebar.cmdClick(a.id); // enter selection mode
    await sidebar.plainClick(b.id); // plain click now toggles, not navigates
    await sidebar.expectSelected(b.id, true);
    await expect(testPage).toHaveURL(new RegExp(`/t/${nav.id}`));
    expect(await sidebar.selectedCount()).toBe(2);
  });

  test("Escape clears the selection", async ({ testPage, apiClient, seedData }) => {
    const [nav, a] = await seedTasks(apiClient, seedData, ["SB Esc Nav", "SB Esc A"]);
    const sidebar = await openSidebarOn(testPage, nav.id);

    await sidebar.cmdClick(a.id);
    await sidebar.expectSelected(a.id, true);

    await testPage.keyboard.press("Escape");
    await sidebar.expectSelected(a.id, false);
    expect(await sidebar.selectedCount()).toBe(0);
  });
});

test.describe("Sidebar selection-aware context menu", () => {
  test("right-clicking a selected row archives the whole selection", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [nav, a, b] = await seedTasks(apiClient, seedData, [
      "SB Arc Nav",
      "SB Arc A",
      "SB Arc B",
    ]);
    const sidebar = await openSidebarOn(testPage, nav.id);

    await sidebar.cmdClick(a.id);
    await sidebar.cmdClick(b.id);
    expect(await sidebar.selectedCount()).toBe(2);

    await sidebar.rightClick(a.id);
    const bulkItem = sidebar.bulkArchiveMenuItem(2);
    await expect(bulkItem).toBeVisible({ timeout: 5_000 });
    await bulkItem.click();

    const confirm = sidebar.bulkArchiveConfirm();
    await expect(confirm).toBeVisible({ timeout: 5_000 });
    await confirm.click();

    await expect(sidebar.row(a.id)).not.toBeVisible({ timeout: 10_000 });
    await expect(sidebar.row(b.id)).not.toBeVisible({ timeout: 5_000 });
  });

  test("right-clicking an unselected row acts on that row only", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const [nav, a, b, c] = await seedTasks(apiClient, seedData, [
      "SB Solo Nav",
      "SB Solo A",
      "SB Solo B",
      "SB Solo C",
    ]);
    const sidebar = await openSidebarOn(testPage, nav.id);

    await sidebar.cmdClick(a.id);
    await sidebar.cmdClick(b.id);
    expect(await sidebar.selectedCount()).toBe(2);

    // Right-click a row that is NOT in the selection.
    await sidebar.rightClick(c.id);

    // The menu must offer the single-task action, not the bulk one.
    await expect(sidebar.singleArchiveMenuItem()).toBeVisible({ timeout: 5_000 });
    await expect(sidebar.bulkArchiveMenuItem(2)).not.toBeVisible();

    // Opening the menu on an unselected row leaves the existing selection intact
    // (asserted while the menu is open — Escape both closes the menu and clears
    // the selection, which is the documented dismiss behaviour).
    await sidebar.expectSelected(a.id, true);
    await sidebar.expectSelected(b.id, true);

    await testPage.keyboard.press("Escape");
  });
});
