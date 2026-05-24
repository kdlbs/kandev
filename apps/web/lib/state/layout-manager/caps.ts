/**
 * Runtime caps for pinned columns (sidebar / right).
 *
 * The previous hard caps (350 sidebar, 450 right) were too strict on wide
 * displays — users wanted to drag the right panel out to half the screen for
 * file review or terminal work. Caps now scale with viewport so wider screens
 * get more room, while small screens still keep the center column usable.
 *
 * Sidebar uses a tighter ratio than the right panel: file-tree / task-list
 * content rarely benefits from more than ~30% of the screen.
 */

const FALLBACK_VIEWPORT = 1440;

const SIDEBAR_RATIO = 0.3;
const SIDEBAR_FLOOR_PX = 350;

const RIGHT_RATIO = 0.7;
const RIGHT_FLOOR_PX = 800;

/** Pixels reserved for the center column + opposite pinned column so the cap
 *  never grows past `viewport - VIEWPORT_RESERVE_PX`. Without this, an 800px
 *  right-cap floor on a 900px viewport would collapse the center to 0. */
const VIEWPORT_RESERVE_PX = 300;

/** Minimum pixel width for any pinned column. Below this the panel becomes
 *  unusable (icons clipped, scrollbars stacked). */
export const LAYOUT_PINNED_MIN_PX = 180;

function getViewport(viewportWidth?: number): number {
  return viewportWidth ?? (typeof window !== "undefined" ? window.innerWidth : FALLBACK_VIEWPORT);
}

function viewportBound(value: number, viewportWidth: number): number {
  // Always leave at least `VIEWPORT_RESERVE_PX` for the rest of the layout,
  // and never go below the per-column min — even on absurdly narrow viewports.
  return Math.max(LAYOUT_PINNED_MIN_PX, Math.min(value, viewportWidth - VIEWPORT_RESERVE_PX));
}

/** Sidebar max width: max(350, viewportWidth * 0.3), bounded by viewport. */
export function computeSidebarMaxPx(viewportWidth?: number): number {
  const vw = getViewport(viewportWidth);
  return viewportBound(Math.max(SIDEBAR_FLOOR_PX, Math.round(vw * SIDEBAR_RATIO)), vw);
}

/** Right pane max width: max(800, viewportWidth * 0.7), bounded by viewport. */
export function computeRightMaxPx(viewportWidth?: number): number {
  const vw = getViewport(viewportWidth);
  return viewportBound(Math.max(RIGHT_FLOOR_PX, Math.round(vw * RIGHT_RATIO)), vw);
}

/** Pick the runtime cap appropriate for a given column ID. Non-sidebar
 *  pinned columns get the right-pane cap. */
export function computePinnedMaxPxFor(columnId: string, viewportWidth?: number): number {
  return columnId === "sidebar"
    ? computeSidebarMaxPx(viewportWidth)
    : computeRightMaxPx(viewportWidth);
}
