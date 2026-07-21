/**
 * Runtime caps for pinned columns (sidebar / right).
 *
 * The previous hard caps (350 sidebar, 450 right) were too strict on wide
 * displays — users wanted to drag the right panel out to half the screen for
 * file review or terminal work. Caps now scale with available layout width so
 * wider workbenches get more room, while small ones keep the center usable.
 *
 * Sidebar uses a tighter ratio than the right panel: file-tree / task-list
 * content rarely benefits from more than ~30% of the screen.
 */

const FALLBACK_VIEWPORT = 1440;

const SIDEBAR_RATIO = 0.3;
const SIDEBAR_FLOOR_PX = 350;

const RIGHT_RATIO = 0.7;
const RIGHT_FLOOR_PX = 800;

const SIDEBAR_REMAINDER_MIN_PX = 300;

/** Preferred minimum for the primary center/chat column when the right panel
 *  is visible. On smaller containers the right panel's own minimum still wins. */
const RIGHT_CENTER_MIN_PX = 480;

/** Minimum pixel width for any pinned column. Below this the panel becomes
 *  unusable (icons clipped, scrollbars stacked). */
export const LAYOUT_PINNED_MIN_PX = 180;

function getAvailableWidth(availableWidth?: number): number {
  return availableWidth ?? (typeof window !== "undefined" ? window.innerWidth : FALLBACK_VIEWPORT);
}

function availableWidthBound(value: number, availableWidth: number, remainder: number): number {
  // Always leave the other columns their reserved width,
  // and never go below the per-column min — even on absurdly narrow viewports.
  return Math.max(LAYOUT_PINNED_MIN_PX, Math.min(value, availableWidth - remainder));
}

/** Sidebar max width: max(350, availableWidth * 0.3), bounded by available width. */
export function computeSidebarMaxPx(availableWidth?: number): number {
  const width = getAvailableWidth(availableWidth);
  return availableWidthBound(
    Math.max(SIDEBAR_FLOOR_PX, Math.round(width * SIDEBAR_RATIO)),
    width,
    SIDEBAR_REMAINDER_MIN_PX,
  );
}

/** Right pane max: max(800, availableWidth * 0.7), bounded by available width.
 *  When the left pinned column is visible, reserve its live width too so the
 *  primary center column retains its preferred minimum. */
export function computeRightMaxPx(availableWidth?: number, sidebarWidth = 0): number {
  const width = getAvailableWidth(availableWidth);
  return availableWidthBound(
    Math.max(RIGHT_FLOOR_PX, Math.round(width * RIGHT_RATIO)),
    width,
    RIGHT_CENTER_MIN_PX + Math.max(0, sidebarWidth),
  );
}

/** Pick the runtime cap appropriate for a given column ID. Non-sidebar
 *  pinned columns get the right-pane cap. */
export function computePinnedMaxPxFor(
  columnId: string,
  availableWidth?: number,
  sidebarWidth?: number,
): number {
  return columnId === "sidebar"
    ? computeSidebarMaxPx(availableWidth)
    : computeRightMaxPx(availableWidth, sidebarWidth);
}
