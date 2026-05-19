import { expect, type Locator } from "@playwright/test";

// Shared layout assertions for mobile / responsive specs. Extracted because
// both onboarding mobile specs need the same overflow + padding checks and
// duplicating the DOM walk caused review churn.

/**
 * Asserts that no descendant of `root` has a right edge past `root`'s right
 * edge. `label` shows up in the failure message so the offending step is
 * identifiable.
 */
export async function assertNoDescendantOverflowsRight(
  root: Locator,
  label = "container",
): Promise<void> {
  const rootBox = await root.boundingBox();
  expect(rootBox, `${label}: root has no bounding box`).not.toBeNull();
  if (!rootBox) return;
  const rootRight = rootBox.x + rootBox.width;

  // One round-trip to keep this cheap on a deep DOM.
  const overflowing: { tag: string; text: string; right: number }[] = await root.evaluate(
    (node, rightArg) => {
      const limit = rightArg as number;
      const results: { tag: string; text: string; right: number }[] = [];
      // SVG elements in HTML documents report `tagName` lowercase (per the
      // SVG namespace) while HTML elements report uppercase, so normalize.
      const skip = new Set(["svg", "path", "circle", "rect", "line", "g"]);
      const all = node.querySelectorAll("*");
      for (const el of all) {
        if (skip.has(el.tagName.toLowerCase())) continue;
        const rect = el.getBoundingClientRect();
        if (rect.width === 0 || rect.height === 0) continue;
        if (rect.right > limit + 1) {
          results.push({
            tag: el.tagName.toLowerCase(),
            text: (el.textContent ?? "").trim().slice(0, 80),
            right: rect.right,
          });
        }
      }
      return results;
    },
    rootRight,
  );

  expect(
    overflowing,
    `${label}: ${overflowing.length} element(s) overflow the right edge (${rootRight.toFixed(
      1,
    )}). First few:\n${overflowing
      .slice(0, 8)
      .map((o) => `  <${o.tag}> right=${o.right.toFixed(1)} text="${o.text}"`)
      .join("\n")}`,
  ).toHaveLength(0);
}

/**
 * Asserts that the first element matching `selector` inside `root` has
 * equal left and right horizontal gaps to `root`'s edges (within 1 px of
 * sub-pixel rounding). Useful for catching scrollbar-gutter style fixes
 * that leave the padding asymmetric.
 */
export async function assertHorizontalPaddingSymmetric(
  root: Locator,
  selector: string,
  label = selector,
): Promise<void> {
  const result = await root.evaluate((node, sel) => {
    const rootRect = node.getBoundingClientRect();
    const el = node.querySelector(sel as string) as HTMLElement | null;
    if (!el) return { missing: true as const };
    const r = el.getBoundingClientRect();
    return {
      missing: false as const,
      leftGap: Math.round(r.left - rootRect.left),
      rightGap: Math.round(rootRect.right - r.right),
    };
  }, selector);
  expect(result.missing, `${label}: selector "${selector}" matched nothing`).toBe(false);
  if (result.missing) return;
  expect(
    Math.abs(result.leftGap - result.rightGap),
    `${label}: leftGap=${result.leftGap}px, rightGap=${result.rightGap}px (selector "${selector}")`,
  ).toBeLessThanOrEqual(1);
}

/**
 * Asserts that the document scrollWidth does not exceed clientWidth — i.e.
 * the page is not horizontally scrollable.
 */
export async function assertNoDocumentHorizontalOverflow(
  page: { evaluate: <T>(fn: () => T) => Promise<T> },
  label = "document",
): Promise<void> {
  const widths = await page.evaluate(() => ({
    scroll: document.documentElement.scrollWidth,
    client: document.documentElement.clientWidth,
  }));
  expect(
    widths.scroll,
    `${label}: scrollWidth (${widths.scroll}) exceeds clientWidth (${widths.client})`,
  ).toBeLessThanOrEqual(widths.client + 1);
}
