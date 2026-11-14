import { expect, type Page } from "@playwright/test";
import type { SystemJob } from "../../lib/types/system";
import { mockStorageAnalysisBarsOverview } from "./storage-maintenance";

const STATUS_CONTROLS = ["storage-analyze-control", "storage-cleanup-control"];

/** Marks idle live regions before their first job update. */
export async function markStorageActionStatusNodes(page: Page) {
  for (const id of STATUS_CONTROLS) {
    const status = page.getByTestId(id).getByRole("status");
    await expect(status).toHaveText("");
    await status.evaluate((node, value) => node.setAttribute("data-retained-node", value), id);
  }
}

/** Checks that job and viewport updates preserve each marked live region. */
export async function expectStorageActionStatusNodesRetained(page: Page) {
  for (const id of STATUS_CONTROLS) {
    await expect(page.getByTestId(id).getByRole("status")).toHaveAttribute(
      "data-retained-node",
      id,
    );
  }
}

/** Exposes operation requests while tests control the jobs' shared polled state. */
export async function mockStorageActionJobs(page: Page) {
  await mockStorageAnalysisBarsOverview(page);
  let state: SystemJob["state"] = "succeeded";
  const jobs = new Map<string, SystemJob["kind"]>();
  const requests = { analyze: 0, run: 0 };
  for (const action of ["analyze", "run"] as const) {
    await page.route(`**/api/v1/system/storage/${action}`, async (route) => {
      requests[action] += 1;
      const id = `storage-layout-${action}-${requests[action]}`;
      jobs.set(id, action === "analyze" ? "storage-analysis" : "storage-cleanup");
      await route.fulfill({ json: { job_id: id }, status: 202 });
    });
  }
  await page.route("**/api/v1/system/jobs/storage-layout-*", async (route) => {
    const id = new URL(route.request().url()).pathname.split("/").at(-1)!;
    await route.fulfill({
      json: {
        id,
        kind: jobs.get(id),
        state,
        started_at: new Date().toISOString(),
        error: state === "failed" ? "Storage layout fixture failure" : undefined,
      },
    });
  });
  return { requests, setState: (next: SystemJob["state"]) => (state = next) };
}

/** Measures the section, action cells, and labels in one browser layout pass. */
async function readActionLayout(page: Page) {
  return page.getByTestId("storage-actions").evaluate((row) => {
    const box = (element: Element) => {
      const { x, y, width, height, right, bottom } = element.getBoundingClientRect();
      return { x, y, width, height, right, bottom };
    };
    const section = row.closest('[data-testid="storage-settings-page"]')!;
    const copy = row.querySelector('[data-testid="storage-actions-description"]')!;
    const group = row.querySelector('[data-testid="storage-actions-buttons"]')!;
    const buttons = Array.from(row.querySelectorAll("button")).map((button) => {
      const cell = button.closest('[data-testid$="-control"]')!;
      const label = button.querySelector("span:last-child")!;
      return {
        button: box(button),
        cell: box(cell),
        wrapper: box(button.parentElement!),
        label: box(label),
        lineHeight: parseFloat(getComputedStyle(label).lineHeight),
        textFits:
          label.scrollWidth <= label.clientWidth + 1 &&
          label.scrollHeight <= label.clientHeight + 1,
      };
    });
    return {
      section: box(section),
      copy: box(copy),
      group: box(group),
      buttons,
      viewport: window.innerWidth,
      rootFont: parseFloat(getComputedStyle(document.documentElement).fontSize),
      touch: window.innerWidth < 768 || matchMedia("(pointer: coarse)").matches,
      pageFits: document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    };
  });
}

/** Checks available-width composition, readable labels, and full-width wrappers. */
export async function expectStorageActionsFit(page: Page) {
  await expect(page.getByTestId("storage-actions")).toBeVisible();
  await expect
    .poll(
      async () => {
        const { section, copy, group, buttons, viewport, rootFont } = await readActionLayout(page);
        const belowCopy = group.y >= copy.bottom;
        const stacked = buttons[1].button.y >= buttons[0].button.bottom;
        return (
          belowCopy === section.width < 52 * rootFont &&
          stacked === (viewport < 768 || section.width < 24 * rootFont)
        );
      },
      { message: "Storage actions follow the available section width and phone composition" },
    )
    .toBe(true);

  const layout = await readActionLayout(page);
  expect(layout.buttons).toHaveLength(2);
  expect(layout.pageFits).toBe(true);
  for (const { button, cell, wrapper, label, lineHeight, textFits } of layout.buttons) {
    expect(button.x).toBeGreaterThanOrEqual(cell.x - 1);
    expect(button.right).toBeLessThanOrEqual(cell.right + 1);
    expect(button.x).toBeGreaterThanOrEqual(layout.section.x - 1);
    expect(button.right).toBeLessThanOrEqual(layout.section.right + 1);
    expect(Math.abs(button.width - cell.width)).toBeLessThanOrEqual(1);
    expect(Math.abs(wrapper.width - cell.width)).toBeLessThanOrEqual(1);
    expect(label.x).toBeGreaterThanOrEqual(button.x);
    expect(label.right).toBeLessThanOrEqual(button.right);
    expect(label.bottom).toBeLessThanOrEqual(button.bottom);
    expect(textFits).toBe(true);
    expect(button.height).toBeGreaterThanOrEqual(layout.touch ? 44 : 28);
    if (!layout.touch && label.height <= lineHeight + 1) {
      expect(Math.abs(button.height - 28)).toBeLessThanOrEqual(1);
    }
  }
  const [analyze, cleanup] = layout.buttons.map(({ button }) => button);
  const gap = cleanup.y >= analyze.bottom ? cleanup.y - analyze.bottom : cleanup.x - analyze.right;
  expect(gap).toBeGreaterThanOrEqual(11);
}

/** Requires the action center to hit the button or one of its descendants. */
export async function expectStorageActionHitTarget(page: Page, id: string) {
  const button = page.getByTestId(id);
  await button.evaluate((element) => element.scrollIntoView({ block: "center" }));
  expect(
    await button.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const target = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2);
      return target !== null && element.contains(target);
    }),
  ).toBe(true);
}
