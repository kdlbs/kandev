import { test, expect } from "../../fixtures/test-base";
import {
  WIDE_VIEWPORT,
  openWideTask,
  expectApproxWidth,
  getColumnSashIndex,
  getDockviewGroupWidth,
  resizeColumnViaSplitview,
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

  test("resize during maximize does not corrupt the pre-maximize width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Edge maximize");
    const before = await resizeColumnViaSplitview(testPage, "right", 600);

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

  test("resize above viewport clamps at the runtime cap", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await openWideTask(testPage, apiClient, seedData, "Edge past-viewport");
    const actual = await resizeColumnViaSplitview(testPage, "right", 9999);
    const cap = Math.round(WIDE_VIEWPORT.width * 0.7);
    expect(actual).toBeLessThanOrEqual(cap + 10);
    expect(actual).toBeGreaterThan(0);
  });

  test("resize after sidebar hidden does not throw", async ({ testPage, apiClient, seedData }) => {
    const session = await openWideTask(testPage, apiClient, seedData, "Edge sidebar hidden");
    const errors: string[] = [];
    testPage.on("pageerror", (err) => errors.push(err.message));

    await testPage.locator("body").click({ position: { x: 5, y: 5 } });
    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${mod}+b`);
    await testPage.waitForTimeout(250);

    await resizeColumnViaSplitview(testPage, "right", 600);
    await session.expectLayoutHealthy();
    expect(errors).toEqual([]);
  });
});
