import { describe, it, expect, afterEach, vi } from "vitest";
import {
  computeSidebarMaxPx,
  computeRightMaxPx,
  computePinnedMaxPxFor,
  LAYOUT_PINNED_MIN_PX,
} from "./caps";

describe("computeSidebarMaxPx", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("floors at 350px on narrow viewports", () => {
    expect(computeSidebarMaxPx(800)).toBe(350);
    expect(computeSidebarMaxPx(100)).toBe(350);
  });

  it("scales to viewport * 0.3 above the floor", () => {
    expect(computeSidebarMaxPx(2000)).toBe(600);
    expect(computeSidebarMaxPx(3000)).toBe(900);
  });

  it("uses 1440 fallback when window is undefined", () => {
    vi.stubGlobal("window", undefined);
    expect(computeSidebarMaxPx()).toBe(Math.round(1440 * 0.3));
  });
});

describe("computeRightMaxPx", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("floors at 800px on narrow viewports", () => {
    expect(computeRightMaxPx(1024)).toBe(800);
    expect(computeRightMaxPx(100)).toBe(800);
  });

  it("scales to viewport * 0.7 above the floor", () => {
    expect(computeRightMaxPx(2000)).toBe(1400);
    expect(computeRightMaxPx(3000)).toBe(2100);
  });

  it("reads window.innerWidth when no argument passed", () => {
    vi.stubGlobal("window", { innerWidth: 1800 } as Window);
    expect(computeRightMaxPx()).toBe(Math.round(1800 * 0.7));
  });
});

describe("computePinnedMaxPxFor", () => {
  it("picks the sidebar cap for the sidebar column", () => {
    expect(computePinnedMaxPxFor("sidebar", 2000)).toBe(600);
  });

  it("picks the right cap for any other column", () => {
    expect(computePinnedMaxPxFor("right", 2000)).toBe(1400);
    expect(computePinnedMaxPxFor("plan", 2000)).toBe(1400);
  });
});

describe("LAYOUT_PINNED_MIN_PX", () => {
  it("keeps pinned panels usable", () => {
    expect(LAYOUT_PINNED_MIN_PX).toBeGreaterThanOrEqual(150);
  });
});
