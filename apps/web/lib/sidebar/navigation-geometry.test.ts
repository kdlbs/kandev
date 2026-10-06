import { describe, it, expect } from "vitest";
import { navigationGeometry, savedNavigationHeight } from "./navigation-geometry";

describe("sidebar navigation split", () => {
  it("preserves space for Tasks and clamps only the rendered height", () => {
    expect(navigationGeometry(400, 500, 300, false)).toEqual({
      height: 276,
      clipped: true,
      expandable: true,
    });
    expect(navigationGeometry(700, 500, 300, false).height).toBe(300);
    expect(navigationGeometry(700, 500, 300, true).height).toBe(500);
  });
  it("fits default content and handles very short viewports", () => {
    expect(navigationGeometry(700, 200, undefined, false).clipped).toBe(false);
    expect(navigationGeometry(80, 500, 90, true).height).toBe(0);
  });
  it("rounds and bounds only deliberate saved resize values", () => {
    expect(savedNavigationHeight(63)).toBe(64);
    expect(savedNavigationHeight(1800)).toBe(1600);
    expect(savedNavigationHeight(100.4)).toBe(100);
  });
});
