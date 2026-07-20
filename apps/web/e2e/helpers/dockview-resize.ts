import { expect, type Page } from "@playwright/test";
import { computeRightMaxPx, computeSidebarMaxPx } from "../../lib/state/layout-manager/caps";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";

/** Bounding-box info Playwright returns. Re-declared to avoid pulling the
 *  full Locator type just for one shape. */
type Box = { x: number; y: number; width: number; height: number };

export const WIDE_VIEWPORT = { width: 1600, height: 900 };

/** Open a fresh task at the given (or wide-default) viewport and wait for
 *  the desktop dockview layout to settle. Shared by every pane-resize spec. */
export async function openWideTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  viewport: { width: number; height: number } = WIDE_VIEWPORT,
): Promise<SessionPage> {
  await page.setViewportSize(viewport);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForDockviewReady();
  await expect(page.getByTestId("dockview-task-layout")).toBeVisible({ timeout: 15_000 });
  return session;
}

/** Read the live pixel width of a dockview group containing a given panel. */
export async function getDockviewGroupWidth(page: Page, panelId: string): Promise<number> {
  return page.evaluate((id) => {
    type Group = { width: number };
    type Panel = { group: Group };
    type Api = { getPanel: (id: string) => Panel | undefined };
    const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
    if (!api) throw new Error("dockview api not exposed");
    const pnl = api.getPanel(id);
    if (!pnl) throw new Error(`panel ${id} not found`);
    return pnl.group.width;
  }, panelId);
}

/** Read the live pixel width of a dockview group by group ID. */
export async function getDockviewGroupWidthById(page: Page, groupId: string): Promise<number> {
  return page.evaluate((id) => {
    type Group = { id: string; width: number };
    type Api = { groups: Group[] };
    const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
    if (!api) throw new Error("dockview api not exposed");
    const g = api.groups.find((grp) => grp.id === id);
    if (!g) throw new Error(`group ${id} not found`);
    return g.width;
  }, groupId);
}

async function sashBoxAt(page: Page, index: number): Promise<Box> {
  const sashes = page.locator(".dv-sash");
  const count = await sashes.count();
  if (count === 0) throw new Error("no .dv-sash elements found");
  if (index >= count) {
    throw new Error(`sash index ${index} out of range (${count} sashes)`);
  }
  const box = await sashes.nth(index).boundingBox();
  if (!box) throw new Error(`sash ${index} has no bounding box`);
  return box;
}

/**
 * Drag a horizontal-direction sash (between two columns) by deltaX pixels.
 * sashIndex is the dockview sash order (0 = left-most). Reserved for tests
 * that exercise real pointer motion (double-click smoke tests, etc.); most
 * resize tests should prefer {@link resizeColumnViaSplitview} for stability
 * in headless CI.
 */
export async function dragHorizontalSash(
  page: Page,
  sashIndex: number,
  deltaX: number,
  steps = 20,
): Promise<void> {
  const box = await sashBoxAt(page, sashIndex);
  const cx = box.x + box.width / 2;
  const cy = box.y + box.height / 2;
  await page.mouse.move(cx, cy);
  await page.mouse.down();
  await page.mouse.move(cx + deltaX, cy, { steps });
  await page.mouse.up();
  // Give the debounced layout-save 350ms to fire so subsequent reload assertions
  // see the new width.
  await page.waitForTimeout(400);
}

/**
 * Programmatically resize a column via dockview's internal splitview API.
 *
 * `targetWidth` is the desired width in pixels; dockview clamps it against
 * the constraints applied by `setConstraints`, so the returned actual width
 * reflects what was permitted (cap-enforcement is what we want to verify).
 *
 * Avoids the flakiness of real pointer motion in headless CI — `page.mouse`
 * drags are sensitive to viewport-overlap and pointer-events targeting,
 * which produce intermittent failures across sharded browser instances.
 */
async function loosenPinnedConstraints(
  page: Page,
  column: "sidebar" | "right",
  runtimeCap: number,
): Promise<void> {
  await page.evaluate(
    ({ col, cap }) => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const api = (window as any).__dockviewApi__;
      if (!api) return;
      const constraint = { maximumWidth: cap, minimumWidth: 50 };
      if (col === "sidebar") {
        api.getPanel("sidebar")?.group.api.setConstraints(constraint);
        return;
      }
      const r = api.getPanel("files") ?? api.getPanel("changes");
      r?.group.api.setConstraints(constraint);
      for (const gid of ["group-right-top", "group-right-bottom"]) {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        api.groups.find((g: any) => g.id === gid)?.api.setConstraints(constraint);
      }
    },
    { col: column, cap: runtimeCap },
  );
}

export async function resizeColumnViaSplitview(
  page: Page,
  column: "sidebar" | "right",
  targetWidth: number,
): Promise<number> {
  // Production locks pinned-column maxWidth to the current width to prevent
  // dockview's proportional rebalance from growing them. Real users bypass
  // that lock via the sash-drag handler (mousedown widens the cap to the
  // runtime container-proportional cap). Tests simulate the same widening —
  // to the same runtime cap, NOT unlimited — so cap-enforcement assertions
  // still work end-to-end. Production uses Dockview's measured width because
  // the app sidebar is outside the workbench.
  const { availableWidth, sidebarWidth } = await page.evaluate(() => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const api = (window as any).__dockviewApi__;
    const sv = api?.component?.gridview?.root?.splitview;
    return {
      availableWidth:
        api?.width ?? document.querySelector<HTMLElement>(".dv-dockview")?.clientWidth ?? 1440,
      sidebarWidth: api?.getPanel("sidebar") && sv?.length >= 3 ? sv.getViewSize(0) : 0,
    };
  });
  const runtimeCap =
    column === "sidebar"
      ? computeSidebarMaxPx(availableWidth)
      : computeRightMaxPx(availableWidth, sidebarWidth);
  await loosenPinnedConstraints(page, column, runtimeCap);
  const result = await page.evaluate(
    ({ col, target }) => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const w = window as any;
      const api = w.__dockviewApi__;
      const sv = api?.component?.gridview?.root?.splitview;
      if (!api || !sv) throw new Error("dockview splitview not exposed");
      if (sv.length < 2) throw new Error("dockview has fewer than 2 columns");
      if (col === "sidebar" && !api.getPanel("sidebar")) {
        throw new Error("cannot resize sidebar when hidden (index 0 is center column)");
      }
      const idx = col === "sidebar" ? 0 : sv.length - 1;
      sv.resizeView(idx, target);
      const actual = sv.getViewSize(idx) as number;
      // Mirror the production sash-drag mouseup behavior: update the pinned
      // target so `enforcePinnedTargets` doesn't restore the old size on the
      // next layout-change tick.
      if (typeof w.__setPinnedTarget__ === "function") {
        w.__setPinnedTarget__(col, actual);
      }
      // The sidebar width is a global pref persisted on a real drag's mouseup.
      // Mirror that so cross-task / reload assertions see the same width.
      if (col === "sidebar" && typeof w.__setGlobalSidebarWidth__ === "function") {
        w.__setGlobalSidebarWidth__(actual);
      }
      // `sv.resizeView` alone does not fire `onDidLayoutChange` in dockview
      // 4.x — without that, the debounced layout-persistence handler never
      // saves the new width. Call the exposed test helper to force-flush the
      // current layout into storage so reload assertions see the new state.
      if (typeof w.__persistDockviewLayout__ === "function") {
        w.__persistDockviewLayout__();
      }
      return actual;
    },
    { col: column, target: targetWidth },
  );
  // Allow the debounced layout persistence to fire.
  await page.waitForTimeout(400);
  return result;
}

/**
 * Return the index of the sash bordering the sidebar / right column.
 *  - sidebar sash: between groups[0] and groups[1] (= sash 0)
 *  - right sash:  between groups[N-2] and groups[N-1] (= last sash)
 */
export async function getColumnSashIndex(page: Page, column: "sidebar" | "right"): Promise<number> {
  if (column === "sidebar") return 0;
  return page.evaluate(() => {
    const count = document.querySelectorAll(".dv-sash").length;
    if (count === 0) throw new Error("no .dv-sash elements found");
    return count - 1;
  });
}

/** Expect a width to be approximately equal (±slack px) to a target. */
export function expectApproxWidth(actual: number, target: number, slack = 8): void {
  expect(
    Math.abs(actual - target) <= slack,
    `width ${actual} not within ±${slack} of target ${target}`,
  ).toBe(true);
}
