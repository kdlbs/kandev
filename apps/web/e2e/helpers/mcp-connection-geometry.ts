import { expect, type Locator } from "@playwright/test";
import { waitForFiniteAnimations } from "./pr-capture";

// @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4, AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.5, AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.19
export async function expectMCPConnectionGeometry(explorer: Locator, touch: boolean) {
  await waitForFiniteAnimations(explorer);
  const list = explorer.getByTestId("mcp-server-list");
  const rows = list.locator('[data-testid^="mcp-server-row-"]');
  expect(await rows.count()).toBeGreaterThanOrEqual(3);
  await expect
    .poll(() =>
      rows.evaluateAll((elements) =>
        elements.every((row, index) => {
          const bounds = row.getBoundingClientRect();
          const next = elements[index + 1]?.getBoundingClientRect();
          return (
            (!next || next.top >= bounds.bottom - 1) &&
            [...row.querySelectorAll("span, svg")].every((child) => {
              const content = child.getBoundingClientRect();
              return (
                content.top >= bounds.top - 1 &&
                content.bottom <= bounds.bottom + 1 &&
                content.left >= bounds.left - 1 &&
                content.right <= bounds.right + 1
              );
            })
          );
        }),
      ),
    )
    .toBe(true);
  if (touch) {
    for (const row of await rows.all())
      expect((await row.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await expectCenteredMCPHeader(explorer);
  } else {
    const alignment = await explorer
      .locator('[data-slot="dialog-title"]')
      .evaluate((element) => getComputedStyle(element).textAlign);
    expect(["left", "start"]).toContain(alignment);
    const close = (await explorer.getByTestId("mcp-explorer-close").boundingBox())!;
    expect(close.width).toBeCloseTo(28, 0);
    expect(close.height).toBeCloseTo(28, 0);
  }
  await rows.last().scrollIntoViewIfNeeded();
  const last = (await rows.last().boundingBox())!;
  const scroller = (await list.boundingBox())!;
  expect(last.y).toBeGreaterThanOrEqual(scroller.y - 1);
  expect(last.y + last.height).toBeLessThanOrEqual(scroller.y + scroller.height + 1);
}

async function expectCenteredMCPHeader(explorer: Locator) {
  for (const slot of ["drawer-title", "drawer-description"]) {
    const copy = explorer.locator(`[data-slot="${slot}"]`);
    await expect
      .poll(async () => {
        const surface = (await explorer.boundingBox())!;
        const bounds = (await copy.boundingBox())!;
        return Math.abs(bounds.x + bounds.width / 2 - surface.x - surface.width / 2);
      })
      .toBeLessThanOrEqual(1);
    await expect(copy).toHaveCSS("text-align", "center");
  }
  const close = (await explorer.getByTestId("mcp-explorer-close").boundingBox())!;
  const surface = (await explorer.boundingBox())!;
  const title = (await explorer.locator('[data-slot="drawer-title"]').boundingBox())!;
  expect(close.width).toBeGreaterThanOrEqual(44);
  expect(close.height).toBeGreaterThanOrEqual(44);
  expect(close.x).toBeGreaterThanOrEqual(title.x + title.width);
  expect(close.x + close.width).toBeLessThanOrEqual(surface.x + surface.width);
  expect(close.y).toBeGreaterThanOrEqual(surface.y);
  expect(close.y + close.height).toBeLessThanOrEqual(surface.y + surface.height);
}
