import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

// WORKSPACE_PICKER (Cmd/Ctrl+E by default) opens the sidebar-header
// workspace switcher with focus already inside the menu, so a workspace can be
// picked with arrows + Enter alone. The shortcut runs on the global app-root
// listener (useAppShortcuts) and drives the picker through the store flag, so
// it also works while the sidebar is collapsed — the rail expands first.

const MODIFIER = process.platform === "darwin" ? "Meta" : "Control";
const WORKSPACE_ITEM_SELECTOR = '[data-testid^="sidebar-workspace-item-"]';

/** Clear focus so the shortcut isn't suppressed by a page-autofocused input. */
async function pressWorkspacePickerShortcut(page: Page): Promise<void> {
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  await page.keyboard.press(`${MODIFIER}+e`);
}

/** Workspace ids in the order the open menu lists them. */
async function listedWorkspaceIds(page: Page): Promise<string[]> {
  const testIds = await page
    .locator(WORKSPACE_ITEM_SELECTOR)
    .evaluateAll((nodes) => nodes.map((node) => node.getAttribute("data-testid") ?? ""));
  return testIds.map((testId) => testId.replace("sidebar-workspace-item-", ""));
}

test.describe("Workspace picker shortcut (global)", () => {
  test("opens the picker and switches workspace with arrows and Enter", async ({
    testPage,
    apiClient,
  }) => {
    const second = await apiClient.createWorkspace("Picker Shortcut Workspace");

    await testPage.goto("/");
    await expect(testPage.getByTestId("sidebar-workspace-trigger")).toBeVisible({
      timeout: 10_000,
    });

    await pressWorkspacePickerShortcut(testPage);

    await expect(testPage.getByTestId("sidebar-workspace-trigger")).toHaveAttribute(
      "data-state",
      "open",
    );
    const workspaceIds = await listedWorkspaceIds(testPage);
    expect(workspaceIds).toContain(second.id);
    expect(workspaceIds.length).toBeGreaterThanOrEqual(2);

    // The controlled store update renders the menu before Radix's focus
    // effect runs. Wait for the menu to own focus before sending navigation
    // keys, otherwise the first ArrowDown can land on the document body on a
    // busy CI runner.
    await expect(testPage.getByRole("menu")).toBeFocused();

    // Focus lands on the menu itself, so the first ArrowDown moves to item 1
    // and the second to item 2 — no mouse needed at any point.
    await testPage.keyboard.press("ArrowDown");
    await expect(testPage.getByTestId(`sidebar-workspace-item-${workspaceIds[0]}`)).toBeFocused();
    await testPage.keyboard.press("ArrowDown");
    await expect(testPage.getByTestId(`sidebar-workspace-item-${workspaceIds[1]}`)).toBeFocused();

    await testPage.keyboard.press("Enter");

    await expect(testPage).toHaveURL(
      (url) => url.searchParams.get("workspaceId") === workspaceIds[1],
      { timeout: 10_000 },
    );
    await expect(testPage.locator(WORKSPACE_ITEM_SELECTOR).first()).not.toBeVisible();
  });

  test("expands a collapsed sidebar before opening the picker", async ({ testPage, apiClient }) => {
    await apiClient.createWorkspace("Collapsed Picker Workspace");

    await testPage.goto("/");
    const sidebar = testPage.getByTestId("app-sidebar");
    await expect(sidebar).toBeVisible({ timeout: 10_000 });

    await testPage.getByRole("button", { name: "Collapse sidebar" }).click();
    await expect(sidebar).toHaveAttribute("data-collapsed", "true");

    await pressWorkspacePickerShortcut(testPage);

    await expect(sidebar).toHaveAttribute("data-collapsed", "false");
    await expect(testPage.getByTestId("sidebar-workspace-trigger")).toHaveAttribute(
      "data-state",
      "open",
    );
    await expect(testPage.locator(WORKSPACE_ITEM_SELECTOR).first()).toBeVisible();
  });

  test("Escape closes the picker and leaves the workspace unchanged", async ({
    testPage,
    apiClient,
  }) => {
    await apiClient.createWorkspace("Escape Picker Workspace");

    await testPage.goto("/");
    await expect(testPage.getByTestId("sidebar-workspace-trigger")).toBeVisible({
      timeout: 10_000,
    });
    const urlBefore = testPage.url();

    await pressWorkspacePickerShortcut(testPage);
    await expect(testPage.locator(WORKSPACE_ITEM_SELECTOR).first()).toBeVisible();

    await testPage.keyboard.press("Escape");

    await expect(testPage.locator(WORKSPACE_ITEM_SELECTOR).first()).not.toBeVisible();
    expect(testPage.url()).toBe(urlBefore);

    // The store flag must have cleared on dismiss, or a second press would be
    // a no-op (open -> open).
    await pressWorkspacePickerShortcut(testPage);
    await expect(testPage.locator(WORKSPACE_ITEM_SELECTOR).first()).toBeVisible();
  });

  test("does nothing below the sidebar's md boundary", async ({ testPage, apiClient }) => {
    await apiClient.createWorkspace("Narrow Viewport Workspace");

    // 700px sits in the 640-767px band: the sidebar (and picker trigger) is
    // display:none, but the menu portals to document.body — without the
    // viewport guard it opened detached at the corner with a scroll lock.
    await testPage.setViewportSize({ width: 700, height: 800 });
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");

    await pressWorkspacePickerShortcut(testPage);

    await expect(testPage.locator(WORKSPACE_ITEM_SELECTOR).first()).not.toBeVisible();
    await expect(testPage.locator("body")).not.toHaveAttribute("data-scroll-locked", /.+/);
  });
});
