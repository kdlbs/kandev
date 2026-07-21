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

  it("hits the 350px floor when vw * 0.3 falls below the minimum", () => {
    // vw=1000 → 30%=300 < floor 350; viewport reserve = 700 (plenty).
    expect(computeSidebarMaxPx(1000)).toBe(350);
  });

  it("scales to viewport * 0.3 above the floor", () => {
    expect(computeSidebarMaxPx(2000)).toBe(600);
    expect(computeSidebarMaxPx(3000)).toBe(900);
  });

  it("never exceeds viewport - reserve so the center column survives", () => {
    // vw=500 → 30%=150 < floor 350; viewport reserve = 200. So 350 clamps to 200.
    // floor below LAYOUT_PINNED_MIN_PX is also enforced.
    expect(computeSidebarMaxPx(500)).toBe(200);
    expect(computeSidebarMaxPx(500)).toBeGreaterThanOrEqual(LAYOUT_PINNED_MIN_PX);
  });

  it("uses 1440 fallback when window is undefined", () => {
    vi.stubGlobal("window", undefined);
    // vw=1440 → 30%=432 > floor 350.
    expect(computeSidebarMaxPx()).toBe(432);
  });
});

describe("computeRightMaxPx", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("hits the 800px floor on mid viewports", () => {
    // vw=1024 → 70%=716 < floor 800; center reserve leaves a 544px cap.
    expect(computeRightMaxPx(1024)).toBe(544);
  });

  it("scales to viewport * 0.7 above the floor on roomy viewports", () => {
    expect(computeRightMaxPx(2000)).toBe(1400);
    expect(computeRightMaxPx(3000)).toBe(2100);
  });

  it("reserves a visible sidebar so the center column retains its comfort width", () => {
    expect(computeRightMaxPx(2000, 600)).toBe(920);
  });

  it("never collapses the center column on narrow viewports", () => {
    // vw=900 → 70%=630 < floor 800; reserve 480 → viewport bound 420.
    expect(computeRightMaxPx(900)).toBe(420);
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

  it("passes the visible sidebar width through to the right-pane cap", () => {
    expect(computePinnedMaxPxFor("right", 2000, 600)).toBe(920);
  });
});

describe("LAYOUT_PINNED_MIN_PX", () => {
  it("keeps pinned panels usable", () => {
    expect(LAYOUT_PINNED_MIN_PX).toBeGreaterThanOrEqual(150);
  });
});
