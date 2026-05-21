import { expect, type Page } from "@playwright/test";

/** Bounding-box info Playwright returns. Re-declared to avoid pulling the
 *  full Locator type just for one shape. */
type Box = { x: number; y: number; width: number; height: number };

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
 * sashIndex is the dockview sash order (0 = left-most). Uses many small mouse
 * moves so dockview's drag listener fires through-out the drag, mirroring real
 * user motion.
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
 * Return the index of the sash bordering the sidebar / right column.
 *  - sidebar sash: between groups[0] and groups[1] (= sash 0)
 *  - right sash:  between groups[N-2] and groups[N-1] (= last sash)
 */
export async function getColumnSashIndex(page: Page, column: "sidebar" | "right"): Promise<number> {
  if (column === "sidebar") return 0;
  return page.evaluate(() => {
    return document.querySelectorAll(".dv-sash").length - 1;
  });
}

/** Pre-populate the pinned-defaults sessionStorage slot before navigation so
 *  the dockview store seeds with the desired widths on first build. */
export async function setPinnedDefaultsViaStorage(
  page: Page,
  defaults: { sidebar?: number; right?: number },
): Promise<void> {
  await page.evaluate((d) => {
    window.sessionStorage.setItem("kandev.dockview.pinned-defaults", JSON.stringify(d));
  }, defaults);
}

/** Read the pinned-defaults sessionStorage slot. */
export async function readPinnedDefaultsFromStorage(
  page: Page,
): Promise<{ sidebar?: number; right?: number }> {
  return page.evaluate(() => {
    const raw = window.sessionStorage.getItem("kandev.dockview.pinned-defaults");
    return raw ? (JSON.parse(raw) as { sidebar?: number; right?: number }) : {};
  });
}

/** Expect a width to be approximately equal (±slack px) to a target. */
export function expectApproxWidth(actual: number, target: number, slack = 8): void {
  expect(
    Math.abs(actual - target) <= slack,
    `width ${actual} not within ±${slack} of target ${target}`,
  ).toBe(true);
}
