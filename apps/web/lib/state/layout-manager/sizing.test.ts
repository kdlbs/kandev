import { beforeEach, describe, expect, it } from "vitest";
import { computeRootAxisSizes, getPinnedWidth } from "./sizing";
import { computeSidebarMaxPx } from "./caps";
import type { LayoutColumn } from "./types";
import { getGlobalSidebarWidth, setGlobalSidebarWidth } from "@/lib/local-storage";

const sidebar = (extra: Partial<LayoutColumn> = {}): LayoutColumn =>
  ({ id: "sidebar", pinned: true, groups: [], ...extra }) as LayoutColumn;
const right = (extra: Partial<LayoutColumn> = {}): LayoutColumn =>
  ({ id: "right", pinned: true, groups: [], ...extra }) as LayoutColumn;

const TOTAL = 1600; // computeSidebarMaxPx(1600) = max(350, 480) bounded = 480

describe("getPinnedWidth — global sidebar width pref", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("uses the stored pref for the sidebar when it fits the screen", () => {
    setGlobalSidebarWidth(400); // < cap 480
    expect(getPinnedWidth(sidebar(), TOTAL, undefined)).toBe(400);
  });

  it("clamps the sidebar pref to the screen cap without mutating storage", () => {
    const cap = computeSidebarMaxPx(TOTAL);
    setGlobalSidebarWidth(900); // > cap → clamp to fit
    expect(getPinnedWidth(sidebar(), TOTAL, undefined)).toBe(cap);
    // Raw value preserved so a wider monitor restores it.
    expect(getGlobalSidebarWidth()).toBe(900);
  });

  it("falls back to the legacy ratio default when no pref is set", () => {
    // ratio 0.25 * 1600 = 400, clamped to the legacy sidebar initial cap (350).
    expect(getGlobalSidebarWidth()).toBeNull();
    expect(getPinnedWidth(sidebar(), TOTAL, undefined)).toBe(350);
  });

  it("lets an explicit override win over the pref", () => {
    setGlobalSidebarWidth(400);
    // maxWidth pins the runtime cap so the override isn't clamped by innerWidth.
    expect(getPinnedWidth(sidebar({ maxWidth: 600 }), TOTAL, 500)).toBe(500);
  });

  it("does NOT apply the sidebar pref to the right column", () => {
    setGlobalSidebarWidth(200);
    // right: ratio 1/3 * 1600 = 533, clamped to legacy right cap (450).
    expect(getPinnedWidth(right(), TOTAL, undefined)).toBe(450);
  });

  it("uses a narrower right pane on laptop-sized workbenches", () => {
    // MacBook Air-style viewport: 1280px total. The unified AppSidebar's
    // 320px default leaves 960px for dockview; 30% gives the right pane a
    // roomier 288px default while retaining space for its tools.
    expect(getPinnedWidth(right(), 960, undefined)).toBe(288);
  });
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe("computeRootAxisSizes", () => {
  const center = (extra: Partial<LayoutColumn> = {}): LayoutColumn =>
    ({ id: "center", groups: [], ...extra }) as LayoutColumn;

  it("sizes a VERTICAL root along the height and ignores horizontal pinning", () => {
    const columns = [center({ width: 300 }), right({ width: 400, maxWidth: 120, minWidth: 100 })];

    const sized = computeRootAxisSizes(columns, "VERTICAL", 1400, 700, new Map([["right", 250]]));

    expect(sized).toEqual({ sizes: [300, 400], orthogonal: 1400 });
  });

  it("scales a complete set of VERTICAL root extents to the available height", () => {
    const columns = [center({ width: 300 }), right({ width: 400 })];

    const sized = computeRootAxisSizes(columns, "VERTICAL", 1400, 350, new Map());

    expect(sized.sizes).toEqual([150, 200]);
  });

  it("keeps the explicit and flexible distribution for incomplete VERTICAL extents", () => {
    const columns = [center({ width: 300 }), right()];

    const sized = computeRootAxisSizes(columns, "VERTICAL", 1400, 700, new Map());

    expect(sized.sizes).toEqual([300, 400]);
  });

  it.each([
    { widths: [600, 600, undefined], expected: [350, 350, 0] },
    { widths: [600, 600, 600, undefined], expected: [233, 234, 233, 0] },
  ])(
    "fits oversubscribed incomplete vertical rows into the available height: $widths",
    ({ widths, expected }) => {
      const columns = widths.map((width) => center({ width }));
      expect(computeRootAxisSizes(columns, "VERTICAL", 1400, 700, new Map()).sizes).toEqual(
        expected,
      );
    },
  );

  it.each([["HORIZONTAL" as const], [undefined]])(
    "keeps the pinned width policy for a %s root",
    (orientation) => {
      const columns = [center(), right({ width: 400 })];

      const sized = computeRootAxisSizes(
        columns,
        orientation,
        1400,
        700,
        new Map([["right", 300]]),
      );

      expect(sized).toEqual({ sizes: [1100, 300], orthogonal: 700 });
    },
  );
});
