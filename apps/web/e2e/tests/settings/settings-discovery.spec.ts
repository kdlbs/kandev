import type { Locator, Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

const APPEARANCE_PATH = "/settings/preferences/appearance";

function settingsSidebar(page: Page) {
  return page.getByTestId("app-sidebar-settings-mode");
}

function settingsResult(scope: Locator, label: string) {
  return scope.getByRole("link").filter({ hasText: new RegExp(`^${label}`) });
}

test.describe("Settings discovery", () => {
  test("gently reflows matching rows and respects reduced motion", async ({ testPage }) => {
    await testPage.goto(APPEARANCE_PATH);
    const sidebar = settingsSidebar(testPage);
    const search = sidebar.getByRole("searchbox", { name: "Search settings" });
    await search.fill("font");

    const motionRows = sidebar.locator('[data-settings-search-motion-key^="item:"]');
    await expect(motionRows).toHaveCount(2);
    await waitForMotionToSettle(motionRows);

    const animatedReorder = await changeSearchAndSampleMotion(search, "terminal font");
    expect(animatedReorder.map((row) => row.key)).toEqual([
      "item:terminal-font-family",
      "item:terminal-font-size",
    ]);
    // FLIP first paints the surviving rows at their old visual positions even though the DOM is
    // already in ranked order. This inversion is the continuity we are testing.
    expect(animatedReorder[0].top).toBeGreaterThan(animatedReorder[1].top);
    expect(animatedReorder.every((row) => row.animations.includes(160))).toBe(true);
    await waitForMotionToSettle(motionRows);
    const settledTops = await motionRows.evaluateAll((rows) =>
      rows.map((row) => row.getBoundingClientRect().top),
    );
    expect(settledTops[0]).toBeLessThan(settledTops[1]);

    await testPage.emulateMedia({ reducedMotion: "reduce" });
    await search.fill("font");
    const reducedMotionReorder = await changeSearchAndSampleMotion(search, "terminal font");
    expect(reducedMotionReorder.every((row) => row.animations.length === 0)).toBe(true);
  });

  test("filters the tree, reveals an exact control, and preserves browser history", async ({
    testPage,
  }) => {
    await testPage.goto(APPEARANCE_PATH);
    const sidebar = settingsSidebar(testPage);
    const search = sidebar.getByRole("searchbox", { name: "Search settings" });
    await expect(search).toBeVisible();
    const [searchBox, navigationRowBox] = await Promise.all([
      search.boundingBox(),
      sidebar.getByRole("link", { name: "Appearance" }).boundingBox(),
    ]);
    expect(searchBox).not.toBeNull();
    expect(navigationRowBox).not.toBeNull();
    expect(Math.abs(searchBox!.height - navigationRowBox!.height)).toBeLessThanOrEqual(2);

    await search.fill("no such setting exists");
    await expect(sidebar.getByText("No matching settings", { exact: true })).toBeVisible();
    await search.press("Escape");
    await expect(search).toHaveValue("");
    await expect(sidebar.getByRole("link", { name: "Appearance" })).toBeVisible();

    await search.fill("terminal font size");
    const result = settingsResult(sidebar, "Terminal Font Size");
    await expect(result).toBeVisible();
    await expect(result).toContainText("Terminal & Editors");
    await expect(sidebar.getByRole("link", { name: "Appearance" })).toHaveCount(0);
    await result.click();

    await expect(testPage).toHaveURL(
      /\/settings\/preferences\/terminal-editors#setting-terminal-font-size$/,
    );
    const target = testPage.locator('[data-settings-target="setting-terminal-font-size"]');
    await expect(target).toHaveAttribute("data-settings-target-highlight", "true");
    await expect(testPage.getByTestId("terminal-font-size-input")).toBeFocused();

    await testPage.goBack();
    await expect(testPage).toHaveURL(new RegExp(`${APPEARANCE_PATH}$`));
    await expect(
      testPage.getByRole("heading", { level: 2, name: "Appearance", exact: true }),
    ).toBeVisible();
  });

  test("targets the current dirty page without opening the leave guard", async ({
    testPage,
    apiClient,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialLayout = initial.settings.changes_panel_layout === "tree" ? "tree" : "flat";
    const nextLayout = initialLayout === "tree" ? "Flat list" : "Tree";

    await testPage.goto(APPEARANCE_PATH);
    await testPage.getByTestId("changes-panel-layout-select").click();
    await testPage.getByRole("option", { name: nextLayout }).click();
    await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();

    const sidebar = settingsSidebar(testPage);
    await sidebar.getByRole("searchbox", { name: "Search settings" }).fill("resource metrics");
    await settingsResult(sidebar, "Resource Metrics").click();

    await expect(testPage).toHaveURL(
      /\/settings\/preferences\/appearance#setting-resource-metrics$/,
    );
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    const target = testPage.locator('[data-settings-target="setting-resource-metrics"]');
    await expect(target).toHaveAttribute("data-settings-target-highlight", "true");
    expect(await target.evaluate((element) => element.contains(document.activeElement))).toBe(true);
    await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();
  });

  test("uses the existing leave guard for a dirty cross-page result", async ({
    testPage,
    apiClient,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialLayout = initial.settings.changes_panel_layout === "tree" ? "tree" : "flat";
    const nextLayout = initialLayout === "tree" ? "Flat list" : "Tree";

    await testPage.goto(APPEARANCE_PATH);
    await testPage.getByTestId("changes-panel-layout-select").click();
    await testPage.getByRole("option", { name: nextLayout }).click();

    const sidebar = settingsSidebar(testPage);
    await sidebar.getByRole("searchbox", { name: "Search settings" }).fill("terminal font size");
    await settingsResult(sidebar, "Terminal Font Size").click();

    const guard = testPage.getByRole("alertdialog", { name: "Save changes before leaving?" });
    await expect(guard).toBeVisible();
    await guard.getByRole("button", { name: "Continue editing" }).click();
    await expect(testPage).toHaveURL(new RegExp(`${APPEARANCE_PATH}$`));
    await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();
  });
});

async function waitForMotionToSettle(rows: Locator) {
  await rows.evaluateAll(async (elements) => {
    await Promise.all(
      elements.flatMap((element) => element.getAnimations().map((animation) => animation.finished)),
    );
  });
}

async function changeSearchAndSampleMotion(search: Locator, query: string) {
  return search.evaluate(async (input, nextQuery) => {
    const valueSetter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    valueSetter?.call(input, nextQuery);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

    return [...document.querySelectorAll<HTMLElement>('[data-settings-search-motion-key^="item:"]')]
      .map((row) => ({
        key: row.dataset.settingsSearchMotionKey,
        top: row.getBoundingClientRect().top,
        animations: row.getAnimations().map((animation) => animation.effect?.getTiming().duration),
      }))
      .filter(
        (row): row is { key: string; top: number; animations: Array<number | CSSNumericValue> } =>
          Boolean(row.key),
      );
  }, query);
}
