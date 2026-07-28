import { computeRightMaxPx } from "./caps";
import { getPinnedWidth } from "./sizing";

const RIGHT_COLUMN = { id: "right", pinned: true, groups: [] };

export function resolveResponsiveRightWidth(
  totalWidth: number | undefined,
  sidebarWidth: number,
  manualWidth: number | null,
  capOverride?: number,
): number {
  const cap = capOverride ?? computeRightMaxPx(totalWidth, sidebarWidth);
  const width =
    totalWidth ??
    (typeof window !== "undefined" && window.innerWidth > 0 ? window.innerWidth : cap);
  const target = manualWidth ?? getPinnedWidth(RIGHT_COLUMN, width, undefined);
  return Math.min(target, cap);
}
