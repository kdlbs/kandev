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

function getViewport(viewportWidth?: number): number {
  return viewportWidth ?? (typeof window !== "undefined" ? window.innerWidth : FALLBACK_VIEWPORT);
}

/** Sidebar max width: max(350, viewportWidth * 0.3). */
export function computeSidebarMaxPx(viewportWidth?: number): number {
  return Math.max(SIDEBAR_FLOOR_PX, Math.round(getViewport(viewportWidth) * SIDEBAR_RATIO));
}

/** Right pane max width: max(800, viewportWidth * 0.7). */
export function computeRightMaxPx(viewportWidth?: number): number {
  return Math.max(RIGHT_FLOOR_PX, Math.round(getViewport(viewportWidth) * RIGHT_RATIO));
}

/** Pick the runtime cap appropriate for a given column ID. Non-sidebar
 *  pinned columns get the right-pane cap. */
export function computePinnedMaxPxFor(columnId: string, viewportWidth?: number): number {
  return columnId === "sidebar"
    ? computeSidebarMaxPx(viewportWidth)
    : computeRightMaxPx(viewportWidth);
}

/** Minimum pixel width for any pinned column. Below this the panel becomes
 *  unusable (icons clipped, scrollbars stacked). */
export const LAYOUT_PINNED_MIN_PX = 180;
