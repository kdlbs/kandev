import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { assertNoHorizontalOverflow } from "../../helpers/session-stream-overload";
import {
  capturePresentation,
  captureThreadSettings,
  seedThreadPresentation,
  startPresentationThread,
} from "./threads-presentation-helpers";

async function chooseLayout(page: Page, value: "Columns" | "Grid") {
  await page.getByTestId("threads-layout-select").click();
  await page.getByRole("option", { name: value, exact: true }).click();
}

// @covers AC-UI-THREADS-SAVED-VIEWS-005.4, AC-UI-THREADS-DECK-004.6
test("keeps keyboard layout selection inside translated settings at compact desktop widths", async ({
  testPage,
  apiClient,
}, testInfo) => {
  const original = await captureThreadSettings(apiClient);
  try {
    await testPage.goto("/threads");
    await expect(testPage.getByTestId("threads-view-picker")).toBeVisible();
    await seedThreadPresentation(apiClient, { layout: "columns" });
    await expect(testPage.getByTestId("threads-view-picker")).toHaveText("Presentation");
    await testPage.evaluate(() => {
      document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
      localStorage.setItem("theme", "dark");
    });
    for (const width of [820, 768]) {
      await testPage.setViewportSize({ width, height: 900 });
      await testPage.goto("/threads");
      await expect(testPage.getByTestId("threads-layout-shortcut")).toHaveCount(0);
      await expect(testPage.getByTestId("threads-view-controls").getByRole("combobox")).toHaveCount(
        0,
      );
      const controls = await testPage.getByTestId("threads-view-controls").boundingBox();
      expect(controls!.x + controls!.width).toBeLessThanOrEqual(width);
      await assertNoHorizontalOverflow(testPage);
      await testPage.getByTestId("threads-view-settings").click();
      if (width === 820) {
        await testPage.getByTestId("threads-layout-select").press("ArrowDown");
        await expect(testPage.getByRole("listbox")).toBeVisible();
        await expect(testPage.getByRole("option").first()).toBeFocused();
        await testPage.keyboard.press("End");
        await expect(testPage.getByRole("option").last()).toBeFocused();
        await testPage.keyboard.press("Enter");
        await expect
          .poll(async () => (await captureThreadSettings(apiClient)).thread_view_draft?.layout)
          .toBe("grid");
      }
      const editor = testPage.getByTestId("threads-view-settings-popover");
      await expect(editor.getByTestId("threads-layout-select")).toContainText(/[À-ž]/);
      await capturePresentation(testPage, testInfo, `compact-display-pseudo-dark-${width}`);
      const box = await editor.boundingBox();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(width);
      expect(await editor.evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(1);
    }
  } finally {
    await apiClient.saveUserSettings(original);
  }
});

// @covers AC-UI-THREADS-SAVED-VIEWS-005.1, AC-UI-THREADS-SAVED-VIEWS-005.3, AC-UI-THREADS-SAVED-VIEWS-005.4, AC-UI-THREADS-SAVED-VIEWS-005.6, AC-UI-THREADS-DECK-004.7
test("saves, discards, copies and reloads both presentation choices without changing task order", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(240_000);
  const original = await captureThreadSettings(apiClient);
  const sidebar = (await apiClient.getUserSettings()).settings.sidebar_views;
  try {
    await testPage.setViewportSize({ width: 1440, height: 1100 });
    for (const name of ["A display settings", "B display settings", "C display settings"]) {
      await startPresentationThread(testPage, apiClient, seedData, name);
    }
    await seedThreadPresentation(apiClient, { layout: "columns" });
    await testPage.goto("/threads");
    const board = testPage.getByTestId("threads-board");
    const tiles = board.locator("[data-thread-column-id]");
    await expect(tiles).toHaveCount(3);
    const order = await tiles.evaluateAll((nodes) =>
      nodes.map((node) => node.getAttribute("data-thread-column-id")),
    );
    await testPage.getByTestId("threads-view-settings").click();
    await chooseLayout(testPage, "Grid");
    const editor = testPage.getByTestId("threads-view-settings-popover");
    await expect(editor).toBeVisible();
    await expect(testPage.getByTestId("threads-view-dirty")).toBeVisible();
    await expect(board).toHaveAttribute("data-layout", "grid");
    await editor.getByRole("switch", { name: "Auto-hide composer" }).click();
    await editor.getByLabel("Maximum chats").fill("3");
    await capturePresentation(testPage, testInfo, "desktop-display-draft");
    await editor.getByTestId("threads-view-save").click();
    await expect
      .poll(async () => (await captureThreadSettings(apiClient)).thread_view_draft)
      .toBeNull();
    expect((await captureThreadSettings(apiClient)).thread_views[0]).toMatchObject({
      layout: "grid",
      auto_hide_composer: true,
      max_columns: 3,
    });
    await testPage.keyboard.press("Escape");
    await testPage.mouse.move(0, 0);
    await expect(board.getByTestId("chat-input-editor").first()).toBeHidden();
    await testPage.reload();
    await expect(board).toHaveAttribute("data-layout", "grid");
    expect(
      await tiles.evaluateAll((nodes) =>
        nodes.map((node) => node.getAttribute("data-thread-column-id")),
      ),
    ).toEqual(order);

    await testPage.setViewportSize({ width: 1440, height: 650 });
    await expect(board).toHaveAttribute("data-layout", "columns");
    await testPage.getByTestId("threads-view-settings").click();
    await expect(editor.getByTestId("threads-layout-select")).toHaveText("Grid");
    await expect(
      editor.getByText("Grid needs more height. Showing Columns.", { exact: true }),
    ).toBeVisible();
    await capturePresentation(testPage, testInfo, "desktop-grid-height-explanation");
    await testPage.setViewportSize({ width: 1440, height: 1100 });
    await chooseLayout(testPage, "Columns");
    await editor.getByRole("switch", { name: "Auto-hide composer" }).click();
    await editor.getByTestId("threads-view-discard").click();
    await expect(editor.getByTestId("threads-layout-select")).toHaveText("Grid");
    await expect(editor.getByRole("switch", { name: "Auto-hide composer" })).toBeChecked();
    await editor.getByLabel("Maximum chats").fill("2");
    await editor.getByTestId("threads-view-save-as").click();
    await editor.getByTestId("threads-view-name-input").fill("Compact monitoring");
    await editor.getByTestId("threads-view-name-input").press("Enter");
    await expect
      .poll(async () => (await captureThreadSettings(apiClient)).thread_views.length)
      .toBe(2);
    await expect(tiles).toHaveCount(2);
    await editor.getByTestId("threads-view-duplicate").click();
    await expect
      .poll(async () => (await captureThreadSettings(apiClient)).thread_views.length)
      .toBe(3);
    const saved = await captureThreadSettings(apiClient);
    expect(
      saved.thread_views.find((view) => view.id === saved.thread_active_view_id),
    ).toMatchObject({ layout: "grid", auto_hide_composer: true, max_columns: 2 });
    expect((await apiClient.getUserSettings()).settings.sidebar_views).toEqual(sidebar);
    await assertNoHorizontalOverflow(testPage);
  } finally {
    await apiClient.saveUserSettings(original);
  }
});

// @covers AC-UI-THREADS-SAVED-VIEWS-005.3, AC-UI-THREADS-SAVED-VIEWS-005.4, AC-UI-THREADS-SAVED-VIEWS-005.7
test("synchronizes another client and retries the latest rejected rapid presentation edit", async ({
  testPage,
  apiClient,
  seedData,
  browser,
}, testInfo) => {
  test.setTimeout(180_000);
  const original = await captureThreadSettings(apiClient);
  let releaseFailure = () => {};
  let secondContext: Awaited<ReturnType<typeof browser.newContext>> | undefined;
  try {
    await testPage.setViewportSize({ width: 1440, height: 1100 });
    await startPresentationThread(testPage, apiClient, seedData, "Synchronized display");
    await seedThreadPresentation(apiClient, { layout: "columns" });
    await testPage.goto("/threads");
    secondContext = await browser.newContext({
      baseURL: new URL(testPage.url()).origin,
      storageState: await testPage.context().storageState(),
      viewport: { width: 1440, height: 1100 },
    });
    const second = await secondContext.newPage();
    await second.goto("/threads");
    await second.getByTestId("threads-view-settings").click();
    await expect(second.getByTestId("threads-layout-select")).toHaveText("Columns");
    await testPage.getByTestId("threads-view-settings").click();
    await chooseLayout(testPage, "Grid");
    await testPage.getByRole("switch", { name: "Auto-hide composer" }).click();
    await expect
      .poll(async () => (await captureThreadSettings(apiClient)).thread_view_draft)
      .toMatchObject({ layout: "grid", auto_hide_composer: true });
    await expect(second.getByTestId("threads-layout-select")).toHaveText("Grid");
    await expect(second.getByRole("switch", { name: "Auto-hide composer" })).toBeChecked();
    await second.getByTestId("threads-view-save").click();
    await expect(testPage.getByTestId("threads-view-dirty")).toHaveCount(0);

    let intercepted = 0;
    const heldFailure = new Promise<void>((resolve) => {
      releaseFailure = resolve;
    });
    await testPage.route("**/api/v1/user/settings", async (route) => {
      if (
        route.request().method() === "PATCH" &&
        intercepted < 2 &&
        route.request().postDataJSON()?.thread_view_draft
      ) {
        intercepted++;
        await heldFailure;
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "Presentation write rejected" }),
        });
      } else await route.continue();
    });
    await chooseLayout(testPage, "Columns");
    await expect.poll(() => intercepted).toBe(1);
    await testPage.getByRole("switch", { name: "Auto-hide composer" }).click();
    releaseFailure();
    await expect.poll(() => intercepted).toBe(2);
    await expect(testPage.getByTestId("threads-view-sync-error")).toBeVisible();
    await expect(testPage.getByTestId("threads-layout-select")).toHaveText("Grid");
    await expect(testPage.getByRole("switch", { name: "Auto-hide composer" })).toBeChecked();
    await capturePresentation(testPage, testInfo, "desktop-display-write-recovery");
    await testPage.getByTestId("threads-view-sync-retry").click();
    await expect
      .poll(async () => (await captureThreadSettings(apiClient)).thread_view_draft)
      .toMatchObject({ layout: "columns", auto_hide_composer: false });
    await expect(second.getByTestId("threads-layout-select")).toHaveText("Columns");
    await expect(second.getByRole("switch", { name: "Auto-hide composer" })).not.toBeChecked();
  } finally {
    releaseFailure();
    await secondContext?.close();
    await testPage.unroute("**/api/v1/user/settings");
    await apiClient.saveUserSettings(original);
  }
});
