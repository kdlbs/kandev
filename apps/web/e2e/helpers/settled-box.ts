import { expect, type Locator } from "@playwright/test";

type BoundingBox = { x: number; y: number; width: number; height: number };

/**
 * Scroll `locator` into view and resolve its bounding box once it stops moving.
 *
 * `scrollIntoViewIfNeeded()` resolves before an animated or momentum scroll has
 * finished, so reading `boundingBox()` straight afterwards can capture
 * coordinates that are already stale by the time a pointer event is dispatched
 * at them. Drag helpers that then aim at those coordinates miss their target
 * intermittently.
 *
 * Poll until two consecutive samples agree rather than sleeping for a budget
 * that is guessed rather than measured: this returns as soon as the element is
 * actually still, and keeps waiting when the scroll is slow.
 */
export async function settledBoundingBox(locator: Locator, timeout = 5_000): Promise<BoundingBox> {
  await locator.scrollIntoViewIfNeeded();

  let previous: string | null = null;
  let latest: BoundingBox | null = null;

  await expect
    .poll(
      async () => {
        latest = await locator.boundingBox();
        const current = latest ? `${Math.round(latest.x)},${Math.round(latest.y)}` : null;
        const settled = current !== null && current === previous;
        previous = current;
        return settled;
      },
      { timeout, message: "element position did not settle" },
    )
    .toBe(true);

  if (!latest) throw new Error("element has no bounding box after settling");
  return latest;
}
