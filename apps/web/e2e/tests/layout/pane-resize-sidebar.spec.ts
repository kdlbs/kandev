import { test, expect } from "../../fixtures/test-base";
import {
  openWideTask,
  expectApproxWidth,
  getDockviewGroupWidth,
  resizeColumnViaSplitview,
} from "../../helpers/dockview-resize";

test.describe("Sidebar resize — viewport-proportional cap", () => {
  test("resizes past the old 350px hard cap", async ({ testPage, apiClient, seedData }) => {
    await openWideTask(testPage, apiClient, seedData, "Sidebar past cap");
    const actual = await resizeColumnViaSplitview(testPage, "sidebar", 480);
    expect(actual).toBeGreaterThan(420);
  });

  test("respects the viewport-proportional cap on narrower screens", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Sidebar cap narrow", {
      width: 1200,
      height: 800,
    });
    const actual = await resizeColumnViaSplitview(testPage, "sidebar", 5000);
    expect(actual).toBeLessThanOrEqual(Math.max(350, Math.round(1200 * 0.3)) + 10);
  });

  test("user width survives reload", async ({ testPage, apiClient, seedData }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Sidebar reload");
    const before = await resizeColumnViaSplitview(testPage, "sidebar", 440);

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();

    const after = await getDockviewGroupWidth(testPage, "sidebar");
    expectApproxWidth(after, before, 12);
  });

  test("toggle sidebar off+on preserves the last user width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Sidebar toggle");
    const before = await resizeColumnViaSplitview(testPage, "sidebar", 430);

    // TOGGLE_SIDEBAR shortcut (Ctrl/Cmd+B). Move focus out of any input first.
    await testPage.locator("body").click({ position: { x: 5, y: 5 } });
    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${mod}+b`);
    await testPage.waitForTimeout(250);
    await testPage.keyboard.press(`${mod}+b`);
    await session.waitForDockviewReady();

    const after = await getDockviewGroupWidth(testPage, "sidebar");
    expectApproxWidth(after, before, 20);
  });
});
