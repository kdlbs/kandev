/**
 * Mobile parity for the sidebar view system (filters / sort / group + saved
 * views).
 *
 * The desktop sidebar exposes a `SidebarFilterBar` (saved-view chip row + a
 * filter gear that opens the shared `SidebarFilterPopover`). That same bar is
 * now folded into the mobile task-switcher sheet so mobile users can switch and
 * edit views instead of being stuck on whatever view was last picked on
 * desktop. The sheet already applied the active view via `applyView`; these
 * tests guard the newly-added switching/editing UI.
 *
 * Lives in `mobile-*.spec.ts` so the `mobile-chrome` Playwright project applies
 * the mobile device automatically.
 */
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { Page, Locator } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

async function seedAndOpenSheet(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  titles: string[],
): Promise<Locator> {
  const stepOpts = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  for (const title of titles) {
    await apiClient.seedTask(seedData.workspaceId, title, stepOpts);
  }
  // The nav task is seeded (not createTask) so it has a primary session and the
  // mobile chat panel renders — `session.waitForLoad()` gates on session-chat.
  const navTask = await apiClient.seedTask(seedData.workspaceId, "Mobile Views Nav", stepOpts);
  await testPage.goto(`/t/${navTask.task_id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  // Open the task-switcher sheet from the mobile session top bar.
  await testPage.getByTestId("mobile-session-menu").click();
  const sheet = testPage.getByRole("dialog");
  await expect(sheet.getByTestId("sidebar-filter-bar")).toBeVisible({ timeout: 10_000 });
  return sheet;
}

/** Add a "Title contains <value>" filter clause via the (portaled) popover. */
async function addTitleFilter(testPage: Page, sheet: Locator, value: string): Promise<void> {
  await sheet.getByTestId("sidebar-filter-gear").click();
  const popover = testPage.getByTestId("sidebar-filter-popover");
  await expect(popover).toBeVisible();
  await popover.getByTestId("filter-add-button").click();
  await popover.getByTestId("filter-dimension-select").click();
  await testPage.getByRole("option", { name: "Title", exact: false }).first().click();
  await popover.getByTestId("filter-value-input").fill(value);
}

test.describe("Mobile sidebar — view system", () => {
  test("editing filters in the mobile sheet narrows the task list live", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sheet = await seedAndOpenSheet(testPage, apiClient, seedData, [
      "Fix auth bug",
      "Update deps",
      "Refactor auth",
    ]);

    // All seeded tasks visible before filtering.
    await expect(sheet.getByText("Fix auth bug")).toBeVisible({ timeout: 10_000 });
    await expect(sheet.getByText("Update deps")).toBeVisible();

    await addTitleFilter(testPage, sheet, "auth");
    // Draft is active — the gear shows its unsaved indicator.
    await expect(testPage.getByTestId("sidebar-filter-gear-indicator")).toBeVisible();
    await testPage.keyboard.press("Escape");

    // The list inside the sheet re-filters live via applyView.
    await expect(sheet.getByText("Fix auth bug")).toBeVisible();
    await expect(sheet.getByText("Refactor auth")).toBeVisible();
    await expect(sheet.getByText("Update deps")).toHaveCount(0);
  });

  test("switching saved views swaps the filtered list in the sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sheet = await seedAndOpenSheet(testPage, apiClient, seedData, [
      "Fix auth bug",
      "Update deps",
    ]);

    // Build a saved "Auth View" that only keeps auth tasks.
    await addTitleFilter(testPage, sheet, "auth");
    const popover = testPage.getByTestId("sidebar-filter-popover");
    await popover.getByTestId("view-save-as-button").click();
    await popover.getByTestId("view-save-as-name-input").fill("Auth View");
    await popover.getByTestId("view-save-as-confirm").click();
    await testPage.keyboard.press("Escape");

    const chipRow = sheet.getByTestId("sidebar-view-chip-row");
    // Auth View is active; non-auth task hidden.
    await expect(
      chipRow.getByTestId("sidebar-view-chip").filter({ hasText: "Auth View" }),
    ).toHaveAttribute("data-active", "true");
    await expect(sheet.getByText("Update deps")).toHaveCount(0);

    // Switch back to the default "All tasks" chip — full list returns.
    await chipRow.getByTestId("sidebar-view-chip").filter({ hasText: "All tasks" }).click();
    await expect(sheet.getByText("Update deps")).toBeVisible();
    await expect(sheet.getByText("Fix auth bug")).toBeVisible();
  });
});
