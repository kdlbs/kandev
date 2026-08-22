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
  const sheet = testPage.getByRole("dialog", { name: "Tasks" });
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

async function taskRowOrder(sheet: Locator, taskIds: string[]) {
  const rows = await sheet.locator("[data-task-row-id]").all();
  const order: string[] = [];
  for (const row of rows) {
    const id = await row.getAttribute("data-task-row-id");
    if (id && taskIds.includes(id)) order.push(id);
  }
  return order;
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
    await expect(sheet.getByTestId("mobile-task-switcher-list")).toHaveCSS("overflow-y", "auto");
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
    const reloadedSheet = testPage.getByRole("dialog", { name: "Tasks" });
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

  test("offers archived as a filter dimension and loads archived rows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const archivedTask = await apiClient.seedTask(seedData.workspaceId, "Mobile archived filter", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.archiveTask(archivedTask.task_id);
    const sheet = await seedAndOpenSheet(testPage, apiClient, seedData, ["Mobile filter options"]);
    await sheet.getByTestId("sidebar-filter-gear").click();
    const popover = testPage.getByTestId("sidebar-filter-popover");
    await expect(popover).toBeVisible();
    await popover.getByTestId("filter-add-button").click();
    await popover.getByTestId("filter-dimension-select").click();

    await expect(testPage.getByRole("option", { name: "Archived", exact: true })).toBeVisible();
    await testPage.getByRole("option", { name: "Archived", exact: true }).click();
    const viewport = testPage.viewportSize();
    const sheetBox = await sheet.boundingBox();
    const popoverBox = await popover.boundingBox();
    expect(viewport).not.toBeNull();
    expect(sheetBox).not.toBeNull();
    expect(popoverBox).not.toBeNull();
    for (const box of [sheetBox!, popoverBox!]) {
      expect(box.x).toBeGreaterThanOrEqual(0);
      expect(box.y).toBeGreaterThanOrEqual(0);
      expect(box.x + box.width).toBeLessThanOrEqual(viewport!.width);
      expect(box.y + box.height).toBeLessThanOrEqual(viewport!.height);
    }
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await testPage.keyboard.press("Escape");
    await expect(sheet.getByText("Mobile archived filter")).toBeVisible({ timeout: 10_000 });
    await expect(sheet.getByText("Archived", { exact: true })).toBeVisible();
    await expect(sheet.getByText("Mobile filter options")).toHaveCount(0);
    await sheet.getByText("Mobile archived filter").click();
    await expect(testPage).toHaveURL((url) => url.pathname === `/t/${archivedTask.task_id}`);
    // The mobile detail header uses the session top bar; desktop renders the
    // unarchive button directly in its task top bar.
    await expect(testPage.getByTestId("mobile-session-menu")).toBeVisible({ timeout: 10_000 });
    await expect(
      testPage
        .getByTestId("mobile-task-layout")
        .getByText("Mobile archived filter", { exact: true }),
    ).toBeVisible();
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

  test("mobile last activity sort is touch reachable, persistent, and contained", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const sheet = await seedAndOpenSheet(testPage, apiClient, seedData, [
      "Mobile activity old",
      "Mobile activity new",
    ]);
    const listed = await apiClient.listTasks(seedData.workspaceId);
    const oldTask = listed.tasks.find((task) => task.title === "Mobile activity old");
    const newTask = listed.tasks.find((task) => task.title === "Mobile activity new");
    expect(oldTask?.id).toBeTruthy();
    expect(newTask?.id).toBeTruthy();
    await apiClient.updateTaskTitle(oldTask!.id, "Mobile activity old touched");

    const gear = sheet.getByTestId("sidebar-filter-gear");
    await gear.tap();
    const popover = testPage.getByTestId("sidebar-filter-popover");
    await expect(popover).toBeVisible();
    await popover.getByTestId("sort-key-select").tap();
    await expect(testPage.getByRole("option", { name: "Updated", exact: true })).toContainText(
      "Last task summary refresh. Background events can change it.",
    );
    await expect(
      testPage.getByRole("option", { name: "Last activity", exact: true }),
    ).toContainText("Last user or agent action. Viewing a task does not change it.");
    await expect(testPage.getByRole("option", { name: "Status", exact: true })).toContainText(
      "Task state, from review to backlog.",
    );
    await expect(testPage.getByRole("option", { name: "Created", exact: true })).toContainText(
      "When the task was created.",
    );
    await expect(testPage.getByRole("option", { name: "Title", exact: true })).toContainText(
      "Task title in alphabetical order.",
    );
    await expect(testPage.getByRole("option", { name: "Custom", exact: true })).toContainText(
      "The manual order you set for tasks.",
    );
    for (const { label, description } of [
      {
        label: "Updated",
        description: "Last task summary refresh. Background events can change it.",
      },
      {
        label: "Last activity",
        description: "Last user or agent action. Viewing a task does not change it.",
      },
      { label: "Status", description: "Task state, from review to backlog." },
      { label: "Created", description: "When the task was created." },
      { label: "Title", description: "Task title in alphabetical order." },
      { label: "Custom", description: "The manual order you set for tasks." },
    ]) {
      const option = testPage.getByRole("option", { name: label, exact: true });
      const descriptionId = await option.getAttribute("aria-describedby");
      expect(descriptionId).toBeTruthy();
      await expect(testPage.locator(`[id="${descriptionId}"]`)).toHaveText(description);
    }
    await testPage.getByRole("option", { name: "Last activity", exact: true }).tap();
    const direction = popover.getByTestId("sort-direction-toggle");
    if ((await direction.getAttribute("data-direction")) !== "desc") await direction.tap();
    await popover.getByTestId("view-save-as-button").tap();
    await popover.getByTestId("view-save-as-name-input").fill("Mobile last activity");
    await popover.getByTestId("view-save-as-confirm").tap();
    await testPage.keyboard.press("Escape");

    const taskIds = [oldTask!.id, newTask!.id];
    await expect.poll(() => taskRowOrder(sheet, taskIds)).toEqual(taskIds);
    const viewport = testPage.viewportSize();
    const list = sheet.getByTestId("mobile-task-switcher-list");
    const sheetBox = await sheet.boundingBox();
    const listBox = await list.boundingBox();
    expect(viewport).not.toBeNull();
    expect(sheetBox).not.toBeNull();
    expect(listBox).not.toBeNull();
    expect(listBox!.x).toBeGreaterThanOrEqual(sheetBox!.x);
    expect(listBox!.x + listBox!.width).toBeLessThanOrEqual(sheetBox!.x + sheetBox!.width);
    await expect(list).toHaveCSS("overflow-y", "auto");
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    await testPage.reload();
    await new SessionPage(testPage).waitForLoad();
    await testPage.getByTestId("mobile-session-menu").tap();
    const reloadedSheet = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(
      reloadedSheet.getByTestId("sidebar-view-chip").filter({ hasText: "Mobile last activity" }),
    ).toHaveAttribute("data-active", "true");
    await expect.poll(() => taskRowOrder(reloadedSheet, taskIds)).toEqual(taskIds);
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
