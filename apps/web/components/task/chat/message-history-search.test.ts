import { describe, expect, it } from "vitest";
import { computeStyle } from "./message-history-search";

describe("computeStyle", () => {
  it("returns null without an anchor rect", () => {
    expect(computeStyle(null, null)).toBeNull();
  });

  it("positions relative to the viewport when there is no containing block (document.body target)", () => {
    const anchorRect = new DOMRect(40, 500, 300, 32);

    const style = computeStyle(anchorRect, null);

    expect(style).toMatchObject({
      position: "fixed",
      left: 40,
      bottom: window.innerHeight - 500 + 8,
    });
  });

  it("clamps left to the 8px minimum when the anchor sits at the viewport edge", () => {
    const anchorRect = new DOMRect(-20, 500, 300, 32);

    const style = computeStyle(anchorRect, null);

    expect(style?.left).toBe(8);
  });

  it("positions relative to a transformed containing block (Quick Chat's DialogContent) instead of the viewport", () => {
    // DialogContent is centered with a permanent translate, so it does not
    // sit at the viewport origin -- its rect has its own left/top/bottom.
    const containerRect = new DOMRect(100, 80, 800, 600);
    // Anchor (the composer) is inside that container, still given in
    // viewport coordinates by getBoundingClientRect().
    const anchorRect = new DOMRect(140, 600, 300, 32);

    const style = computeStyle(anchorRect, containerRect);

    // Origin-relative: anchorRect.left - containerRect.left, and
    // containerRect.bottom - anchorRect.top + 8, not viewport-relative.
    expect(style).toMatchObject({
      position: "fixed",
      left: 140 - 100,
      bottom: 680 - 600 + 8,
    });
  });

  it("matches the viewport-relative formula when containerRect is null (regression guard for the document.body case)", () => {
    const anchorRect = new DOMRect(40, 500, 300, 32);

    const viewportRelative = computeStyle(anchorRect, null);
    const explicitViewportOrigin = computeStyle(
      anchorRect,
      new DOMRect(0, 0, window.innerWidth, window.innerHeight),
    );

    expect(viewportRelative).toEqual(explicitViewportOrigin);
  });
});
