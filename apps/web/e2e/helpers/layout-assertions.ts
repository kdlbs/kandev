import { expect, type Locator } from "@playwright/test";

export type ElementBox = { x: number; y: number; width: number; height: number };

export async function requireBox(locator: Locator, label: string): Promise<ElementBox> {
  const box = await locator.boundingBox();
  expect(box, `${label}: locator has no bounding box`).not.toBeNull();
  if (!box) throw new Error(`${label}: locator has no bounding box`);
  return box;
}

/**
 * Returns descendant text in the order Chromium paints its glyphs from left to
 * right. DOM text remains in logical order when bidi layout moves punctuation,
 * so each character needs its own rendered range for visual-order assertions.
 */
export async function getSingleLineTextInVisualOrder(locator: Locator): Promise<string> {
  return locator.evaluate((node) => {
    const characters: { character: string; left: number; top: number; sourceIndex: number }[] = [];
    const walker = document.createTreeWalker(node, NodeFilter.SHOW_TEXT);
    let sourceIndex = 0;

    for (let textNode = walker.nextNode(); textNode; textNode = walker.nextNode()) {
      const text = textNode.textContent ?? "";
      let offset = 0;
      for (const character of Array.from(text)) {
        const range = document.createRange();
        const nextOffset = offset + character.length;
        range.setStart(textNode, offset);
        range.setEnd(textNode, nextOffset);
        const rect = range.getBoundingClientRect();
        if (rect.width > 0 && rect.height > 0) {
          characters.push({ character, left: rect.left, top: rect.top, sourceIndex });
        }
        offset = nextOffset;
        sourceIndex += 1;
      }
    }

    const firstTop = characters[0]?.top;
    if (firstTop !== undefined && characters.some(({ top }) => Math.abs(top - firstTop) > 1)) {
      throw new Error("Expected text to render on one line");
    }

    return characters
      .sort((left, right) => left.left - right.left || left.sourceIndex - right.sourceIndex)
      .map(({ character }) => character)
      .join("");
  });
}

// Shared layout assertions for mobile / responsive specs. Extracted because
// both onboarding mobile specs need the same overflow + padding checks and
// duplicating the DOM walk caused review churn.

/**
 * Asserts that no visible descendant of `root` has a right edge past `root`'s
 * right edge. DOM bounds that are clipped inside a nested scroll container do
 * not constitute a visual overflow. `label` shows up in the failure message
 * so the offending step is identifiable.
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
      const clippingOverflow = new Set(["auto", "clip", "hidden", "scroll"]);
      const isClippedBeforeLimit = (element: Element): boolean => {
        for (
          let parent = element.parentElement;
          parent && parent !== node;
          parent = parent.parentElement
        ) {
          const overflowX = getComputedStyle(parent).overflowX;
          if (!clippingOverflow.has(overflowX)) continue;
          if (parent.getBoundingClientRect().right <= limit + 1) return true;
        }
        return false;
      };
      const all = node.querySelectorAll("*");
      for (const el of all) {
        if (skip.has(el.tagName.toLowerCase())) continue;
        const rect = el.getBoundingClientRect();
        if (rect.width === 0 || rect.height === 0) continue;
        if (rect.right > limit + 1 && !isClippedBeforeLimit(el)) {
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

/**
 * Asserts that an element's rendered contents do not exceed its own horizontal
 * content box. Unlike bounding-box checks, this catches overflowing text glyphs.
 */
export async function assertNoElementHorizontalOverflow(
  locator: Locator,
  label = "element",
): Promise<void> {
  const widths = await locator.evaluate((node) => ({
    scroll: node.scrollWidth,
    client: node.clientWidth,
  }));
  expect(
    widths.scroll,
    `${label}: scrollWidth (${widths.scroll}) exceeds clientWidth (${widths.client})`,
  ).toBeLessThanOrEqual(widths.client + 1);
}

/**
 * Asserts that text occupies multiple rendered lines without overflowing its
 * own horizontal content box. Useful for titles whose complete text must stay
 * readable instead of being clipped with an ellipsis.
 */
export async function assertTextWrapsNaturallyWithoutHorizontalOverflow(
  locator: Locator,
  label = "text",
): Promise<void> {
  const metrics = await locator.evaluate((node) => {
    const style = getComputedStyle(node);
    const range = document.createRange();
    range.selectNodeContents(node);
    const lineWidths = Array.from(range.getClientRects(), (rect) => rect.width).filter(
      (width) => width > 0,
    );
    return {
      height: node.getBoundingClientRect().height,
      lineHeight: Number.parseFloat(style.lineHeight),
      scrollWidth: node.scrollWidth,
      clientWidth: node.clientWidth,
      lineWidths,
    };
  });
  expect(
    metrics.height,
    `${label}: height (${metrics.height}) does not span multiple ${metrics.lineHeight}px lines`,
  ).toBeGreaterThan(metrics.lineHeight * 1.5);
  expect(
    metrics.scrollWidth,
    `${label}: scrollWidth (${metrics.scrollWidth}) exceeds clientWidth (${metrics.clientWidth})`,
  ).toBeLessThanOrEqual(metrics.clientWidth + 1);
  const [firstLineWidth] = metrics.lineWidths;
  if (firstLineWidth === undefined) {
    throw new Error(`${label}: text has no rendered line rectangles`);
  }
  expect(
    firstLineWidth,
    `${label}: first line should consume available width before wrapping`,
  ).toBeGreaterThan(metrics.clientWidth * 0.8);
}

/**
 * Asserts that a visible locator fits inside the viewport horizontally.
 * Useful for popovers that portal outside their dialog/container.
 */
export async function assertLocatorWithinViewportX(
  locator: Locator,
  label = "element",
): Promise<void> {
  const box = await locator.boundingBox();
  expect(box, `${label}: locator has no bounding box`).not.toBeNull();
  if (!box) return;
  const viewport = locator.page().viewportSize();
  expect(viewport, `${label}: page has no viewport`).not.toBeNull();
  if (!viewport) return;
  expect(
    box.x,
    `${label}: left edge ${box.x.toFixed(1)} is outside viewport`,
  ).toBeGreaterThanOrEqual(-1);
  expect(
    box.x + box.width,
    `${label}: right edge ${(box.x + box.width).toFixed(1)} exceeds viewport ${viewport.width}`,
  ).toBeLessThanOrEqual(viewport.width + 1);
}

export async function expectElementsNotToIntersect(first: Locator, second: Locator): Promise<void> {
  const [firstBox, secondBox] = await Promise.all([first.boundingBox(), second.boundingBox()]);
  expect(firstBox).not.toBeNull();
  expect(secondBox).not.toBeNull();
  const intersects =
    firstBox!.x < secondBox!.x + secondBox!.width &&
    firstBox!.x + firstBox!.width > secondBox!.x &&
    firstBox!.y < secondBox!.y + secondBox!.height &&
    firstBox!.y + firstBox!.height > secondBox!.y;
  expect(intersects).toBe(false);
}
