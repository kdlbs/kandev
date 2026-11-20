import { computeRightMaxPx } from "./caps";
import { getPinnedWidth } from "./sizing";
import { getReusableRightWidth } from "./pinned-targets";

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

/**
 * Right column width for an environment: an actual manual width, else the
 * environment's owned reusable-layout width, else the stock responsive default;
 * always under the runtime cap.
 */
export function resolveEnvRightWidth(
  totalWidth: number | undefined,
  sidebarWidth: number,
  manualWidth: number | null,
  envId: string | null | undefined,
  capOverride?: number,
): number {
  return resolveResponsiveRightWidth(
    totalWidth,
    sidebarWidth,
    manualWidth ?? getReusableRightWidth(envId, totalWidth),
    capOverride,
  );
}
