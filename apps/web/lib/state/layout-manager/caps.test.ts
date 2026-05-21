import { describe, it, expect, afterEach, vi } from "vitest";
import { computePinnedMaxPx, LAYOUT_PINNED_MIN_PX } from "./caps";

describe("computePinnedMaxPx", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("floors at 800px on narrow viewports", () => {
    expect(computePinnedMaxPx(1024)).toBe(800);
    expect(computePinnedMaxPx(100)).toBe(800);
  });

  it("scales to viewport * 0.7 above the floor", () => {
    expect(computePinnedMaxPx(2000)).toBe(1400);
    expect(computePinnedMaxPx(3000)).toBe(2100);
  });

  it("reads window.innerWidth when no argument passed", () => {
    vi.stubGlobal("window", { innerWidth: 1800 } as Window);
    expect(computePinnedMaxPx()).toBe(Math.round(1800 * 0.7));
  });

  it("uses 1440 fallback when window is undefined", () => {
    vi.stubGlobal("window", undefined);
    expect(computePinnedMaxPx()).toBe(Math.round(1440 * 0.7));
  });
});

describe("LAYOUT_PINNED_MIN_PX", () => {
  it("keeps pinned panels usable", () => {
    expect(LAYOUT_PINNED_MIN_PX).toBeGreaterThanOrEqual(150);
  });
});
