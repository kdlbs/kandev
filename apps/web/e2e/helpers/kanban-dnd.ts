import type { Locator, Page } from "@playwright/test";

/**
 * Pointer drag for @dnd-kit Kanban cards (PointerSensor distance: 8).
 * Playwright's locator.dragTo often skips the activation movement dnd-kit needs.
 */
export async function dragKanbanCard(
  page: Page,
  source: Locator,
  target: Locator,
  opts?: { place?: "above" | "below" },
): Promise<void> {
  const place = opts?.place ?? "below";
  const src = await source.boundingBox();
  const dst = await target.boundingBox();
  if (!src || !dst) {
    throw new Error("dragKanbanCard: missing bounding box");
  }

  const startX = src.x + src.width / 2;
  const startY = src.y + src.height / 2;
  const endX = dst.x + dst.width / 2;
  const endY = place === "below" ? dst.y + dst.height - 4 : dst.y + 4;

  await page.mouse.move(startX, startY);
  await page.mouse.down();
  // Exceed PointerSensor activation distance before aiming at the drop target.
  await page.mouse.move(startX, startY + 12, { steps: 4 });
  await page.mouse.move(endX, endY, { steps: 12 });
  await page.mouse.up();
}
