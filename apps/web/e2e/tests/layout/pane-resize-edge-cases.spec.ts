import { test, expect } from "../../fixtures/test-base";
import {
  WIDE_VIEWPORT,
  openWideTask,
  dragHorizontalSash,
  expectApproxWidth,
  getColumnSashIndex,
  getDockviewGroupWidth,
} from "../../helpers/dockview-resize";

test.describe("Pane resize edge cases", () => {
  test("double-click on sidebar sash does not crash dockview", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Edge dblclick");
    const sashIdx = await getColumnSashIndex(testPage, "sidebar");
    const sash = testPage.locator(".dv-sash").nth(sashIdx);
    await sash.dblclick();
    await session.expectLayoutHealthy();
  });

  test("rapid drags persist the final value across reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Edge rapid drag");
    const sashIdx = await getColumnSashIndex(testPage, "right");
    // Five quick drags, each shifting +60px left from the current position.
    for (let i = 0; i < 5; i++) {
      await dragHorizontalSash(testPage, sashIdx, -60, 5);
    }
    const finalWidth = await getDockviewGroupWidth(testPage, "files");

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForDockviewReady();

    const afterReload = await getDockviewGroupWidth(testPage, "files");
    expectApproxWidth(afterReload, finalWidth, 15);
  });

  test("resize during maximize does not corrupt the pre-maximize width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Edge maximize");
    const sashIdx = await getColumnSashIndex(testPage, "right");
    await dragHorizontalSash(testPage, sashIdx, -250);
    const before = await getDockviewGroupWidth(testPage, "files");

    // Maximize the files group, then exit. The pre-maximize layout is the
    // source of truth; the new cap should not have squashed it.
    await testPage.evaluate(() => {
      type GroupApi = { maximize: () => void };
      type Group = { id: string; panels: { id: string }[]; api: GroupApi };
      type Api = { groups: Group[] };
      const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
      if (!api) throw new Error("dockview api not exposed");
      const matching = api.groups.find((g) => g.panels.some((p) => p.id === "files"));
      if (!matching) throw new Error("files group not found");
      matching.api.maximize();
    });
    await testPage.waitForTimeout(150);
    await testPage.evaluate(() => {
      type GroupApi = { exitMaximized: () => void };
      type Group = { id: string; panels: { id: string }[]; api: GroupApi };
      type Api = { groups: Group[] };
      const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
      if (!api) throw new Error("dockview api not exposed");
      const matching = api.groups.find((g) => g.panels.some((p) => p.id === "files"));
      if (!matching) throw new Error("files group not found");
      matching.api.exitMaximized();
    });
    await session.waitForDockviewReady();

    const after = await getDockviewGroupWidth(testPage, "files");
    expectApproxWidth(after, before, 30);
  });

  test("drag past viewport edge clamps at the runtime cap", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Edge past-viewport");
    const sashIdx = await getColumnSashIndex(testPage, "right");
    await dragHorizontalSash(testPage, sashIdx, -5000);
    const cap = Math.round(WIDE_VIEWPORT.width * 0.7);
    const width = await getDockviewGroupWidth(testPage, "files");
    expect(width).toBeLessThanOrEqual(cap + 10);
    expect(width).toBeGreaterThan(0);
  });

  test("resize after sidebar hidden does not throw", async ({ testPage, apiClient, seedData }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Edge sidebar hidden");
    const errors: string[] = [];
    testPage.on("pageerror", (err) => errors.push(err.message));

    await testPage.locator("body").click({ position: { x: 5, y: 5 } });
    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${mod}+b`);
    await testPage.waitForTimeout(200);

    // The right sash is now between center and right (sash index 0 since
    // sidebar is gone).
    const sashIdx = await getColumnSashIndex(testPage, "right");
    await dragHorizontalSash(testPage, sashIdx, -200);
    await session.expectLayoutHealthy();
    expect(errors).toEqual([]);
  });
});
