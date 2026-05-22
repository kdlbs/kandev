import { test, expect } from "../../fixtures/test-base";
import {
  WIDE_VIEWPORT,
  openWideTask,
  expectApproxWidth,
  getDockviewGroupWidth,
  resizeColumnViaSplitview,
} from "../../helpers/dockview-resize";

test.describe("Right pane resize — viewport-proportional cap", () => {
  test("resizes past the old 450px hard cap", async ({ testPage, apiClient, seedData }) => {
    await openWideTask(testPage, apiClient, seedData, "Right resize past old cap");
    const actual = await resizeColumnViaSplitview(testPage, "right", 700);
    expect(actual).toBeGreaterThan(600);
  });

  test("respects the viewport-proportional cap (max(800, vw*0.7))", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Right cap respect");
    const actual = await resizeColumnViaSplitview(testPage, "right", 5000);
    const cap = Math.round(WIDE_VIEWPORT.width * 0.7);
    expect(actual).toBeLessThanOrEqual(cap + 10);
  });

  test("user width survives reload (per-env sessionStorage round-trip)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Right resize reload");
    const before = await resizeColumnViaSplitview(testPage, "right", 600);

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();

    const after = await getDockviewGroupWidth(testPage, "files");
    expectApproxWidth(after, before, 12);
  });

  test("viewport shrink re-clamps an over-cap pinned width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Right viewport shrink");
    const wideWidth = await resizeColumnViaSplitview(testPage, "right", 900);
    expect(wideWidth).toBeGreaterThan(700);

    await testPage.setViewportSize({ width: 1100, height: 800 });
    // Allow ResizeObserver tick + applyDynamicConstraints to fire, then attempt
    // a re-resize that would exceed the new cap.
    await testPage.waitForTimeout(300);
    const narrowWidth = await resizeColumnViaSplitview(testPage, "right", 1500);

    const newCap = Math.max(800, Math.round(1100 * 0.7));
    expect(narrowWidth).toBeLessThanOrEqual(newCap + 10);
  });
});
