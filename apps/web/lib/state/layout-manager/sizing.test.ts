import { beforeEach, describe, expect, it } from "vitest";
import { getPinnedWidth } from "./sizing";
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
    // right: ratio 0.25 * 1600 = 400, clamped to legacy right cap (450) → 400.
    expect(getPinnedWidth(right(), TOTAL, undefined)).toBe(400);
  });
});
