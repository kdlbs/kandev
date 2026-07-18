/**
 * Mobile parity for the sidebar view system (filters / sort / group + saved
 * views).
 *
 * The mobile task-switcher sheet exposes a saved-view chip row, fixed direct
 * creation action, and filter gear backed by the shared `SidebarFilterPopover`.
 * The desktop AppSidebar uses a separate dropdown picker for the same state.
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
  // Radix Select portals options to document.body (not under `popover`), so we
  // can't scope to the popover here; `.first()` is the deliberate convention for
  // this case (see apps/web/AGENTS.md). Only one select is open at a time.
  await testPage.getByRole("option", { name: "Title", exact: false }).first().click();
  await popover.getByTestId("filter-value-input").fill(value);
}

test.describe("Mobile sidebar — view system", () => {
  test("direct creation stays touch-reachable, viewport-safe, and persists", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sheet = await seedAndOpenSheet(testPage, apiClient, seedData, ["Mobile New View Task"]);
    const newView = sheet.getByTestId("sidebar-new-view");
    const gear = sheet.getByTestId("sidebar-filter-gear");
    await expect(newView).toBeVisible();
    await expect(newView).toContainText("New view");
    const newViewBox = await newView.boundingBox();
    const gearBox = await gear.boundingBox();
    const chipRowBox = await sheet.getByTestId("sidebar-view-chip-row").boundingBox();
    expect(newViewBox?.height).toBeGreaterThanOrEqual(40);
    expect(gearBox?.height).toBeGreaterThanOrEqual(40);
    expect(chipRowBox!.x + chipRowBox!.width).toBeLessThanOrEqual(newViewBox!.x);
    expect(newViewBox!.x + newViewBox!.width).toBeLessThanOrEqual(gearBox!.x);
    await expect(
      sheet.getByTestId("sidebar-view-chip-row").getByTestId("sidebar-new-view"),
    ).toHaveCount(0);

    await newView.click();
    const popover = testPage.getByTestId("sidebar-filter-popover");
    const renameInput = popover.getByTestId("view-rename-input");
    await expect(popover).toBeVisible();
    await expect(renameInput).toBeFocused();
    await expect(renameInput).toHaveValue("New view");
    const popoverBox = await popover.boundingBox();
    const viewport = testPage.viewportSize();
    expect(popoverBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(popoverBox!.x).toBeGreaterThanOrEqual(0);
    expect(popoverBox!.x + popoverBox!.width).toBeLessThanOrEqual(viewport!.width);

    await popover.getByRole("button", { name: "Cancel" }).click();
    await testPage.keyboard.press("Escape");
    await expect(
      sheet.getByTestId("sidebar-view-chip-row").getByTestId("sidebar-view-chip").filter({
        hasText: "New view",
      }),
    ).toHaveAttribute("data-active", "true");
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await expect
      .poll(async () => {
        const { settings } = await apiClient.getUserSettings();
        return (settings.sidebar_views as Array<{ name?: string }> | undefined)?.some(
          (view) => view.name === "New view",
        );
      })
      .toBe(true);

    await testPage.reload();
    await new SessionPage(testPage).waitForLoad();
    await testPage.getByTestId("mobile-session-menu").click();
    const reloadedSheet = testPage.getByRole("dialog");
    await expect(
      reloadedSheet
        .getByTestId("sidebar-view-chip-row")
        .getByTestId("sidebar-view-chip")
        .filter({ hasText: "New view" }),
    ).toHaveAttribute("data-active", "true");
  });

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
    // Draft is active — the gear shows its unsaved indicator. Scope to the sheet:
    // the globally-mounted (hidden on mobile) AppSidebar TasksViewPicker renders
    // the same testid, so a page-level query is a strict-mode collision.
    await expect(sheet.getByTestId("sidebar-filter-gear-indicator")).toBeVisible();
    const blockedNewView = sheet.getByTestId("sidebar-new-view");
    await expect(blockedNewView).toHaveAttribute("aria-disabled", "true");
    await expect(blockedNewView).toHaveAttribute("aria-label", /save or discard changes/i);
    // aria-disabled communicates the blocked state, but this action remains
    // clickable so touch/keyboard users can get the concrete reason toast.
    await blockedNewView.click({ force: true });
    await expect(
      testPage.getByText("Save or discard changes before creating a new view."),
    ).toBeVisible();
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

  test("many saved views scroll without covering fixed actions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedAndOpenSheet(testPage, apiClient, seedData, ["Scrollable Views Task"]);
    const views = Array.from({ length: 8 }, (_, index) => ({
      id: `mobile-scroll-${index}`,
      name: `Long mobile view ${index + 1}`,
      filters: [],
      sort: { key: "state", direction: "asc" },
      group: "repository",
      collapsed_groups: [],
    }));
    const response = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      sidebar_views: views,
      sidebar_active_view_id: views[0].id,
      sidebar_draft: null,
    });
    expect(response.ok).toBe(true);

    await testPage.reload();
    await new SessionPage(testPage).waitForLoad();
    await testPage.getByTestId("mobile-session-menu").click();
    const sheet = testPage.getByRole("dialog");
    const chipRow = sheet.getByTestId("sidebar-view-chip-row");
    await expect(chipRow).toBeVisible();
    expect(await chipRow.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(
      true,
    );
    await chipRow.evaluate((element) => {
      element.scrollLeft = element.scrollWidth;
    });
    expect(await chipRow.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
    await expect(sheet.getByTestId("sidebar-new-view")).toBeVisible();
    await expect(sheet.getByTestId("sidebar-filter-gear")).toBeVisible();
  });

  test("tapping a group header collapses and expands the group in the sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // The default "All tasks" view groups by repository, so the seeded tasks
    // render under a collapsible group header in the sheet.
    const sheet = await seedAndOpenSheet(testPage, apiClient, seedData, [
      "Collapse Task A",
      "Collapse Task B",
    ]);

    const header = sheet.getByTestId("sidebar-group-header").first();
    await expect(header).toBeVisible();
    await expect(sheet.getByText("Collapse Task A")).toBeVisible();

    // Collapse hides the group's tasks; expand brings them back.
    await header.click();
    await expect(sheet.getByText("Collapse Task A")).toHaveCount(0);
    await header.click();
    await expect(sheet.getByText("Collapse Task A")).toBeVisible();
  });
});
