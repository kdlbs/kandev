import { test, expect } from "../../fixtures/test-base";
import {
  WIDE_VIEWPORT,
  openWideTask,
  expectApproxWidth,
  getDockviewGroupWidth,
  resizeColumnViaSplitview,
} from "../../helpers/dockview-resize";

// The "double-click on sidebar sash does not crash dockview" test was removed:
// the dockview-embedded sidebar column was retired in the unified AppSidebar
// overhaul, so there is no sidebar sash to interact with anymore. The remaining
// edge cases exercise the RIGHT (files/changes/terminal) column, which is still
// resizable.
test.describe("Pane resize edge cases", () => {
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
    // Wait for the maximized layout to be applied rather than for a number.
    // `hasMaximizedGroup()` flips synchronously inside `maximize()`, so polling
    // it would guard nothing; the group growing is the consequence that has to
    // land before exiting is a real round trip. Measured, maximize takes this
    // group from the 600px set above to ~1000px -- it does not fill the
    // container, so this asserts growth rather than a proportion.
    await expect
      .poll(() => getDockviewGroupWidth(testPage, "files"), {
        timeout: 5_000,
        message: `files group never grew past its pre-maximize ${before}px`,
      })
      .toBeGreaterThan(before + 50);
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

    // This used to press Ctrl+B and sleep 250ms. Ctrl+B does nothing:
    // `CONFIGURABLE_SHORTCUTS.TOGGLE_SIDEBAR` defaults to `UNBOUND_SHORTCUT`,
    // so the binding exists only for users who set one. Nothing downstream
    // asserted on the sidebar, so the test has been resizing with the sidebar
    // still open -- a duplicate of the case above rather than the edge case it
    // is named for. Collapse it through the store instead; the subject here is
    // the resize, not how the collapse was triggered.
    await testPage.evaluate(() => {
      type StoreWindow = Window & {
        __KANDEV_E2E_STORE__?: {
          getState: () => { setAppSidebarCollapsed: (collapsed: boolean) => void };
        };
      };
      const store = (window as StoreWindow).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge missing");
      store.getState().setAppSidebarCollapsed(true);
    });
    // Assert the collapse instead of budgeting for it: this fails if the flag
    // stops reaching the DOM, where the old sleep could not. The 300ms width
    // animation that follows is deliberately not waited out -- the assertions
    // below are "no page error" and "layout still healthy", neither of which
    // reads geometry.
    await expect(testPage.getByTestId("app-sidebar")).toHaveAttribute("data-collapsed", "true", {
      timeout: 5_000,
    });

    await resizeColumnViaSplitview(testPage, "right", 600);
    await session.expectLayoutHealthy();
    expect(errors).toEqual([]);
  });
});
