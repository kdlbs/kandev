/**
 * Runtime caps for pinned columns (sidebar / right).
 *
 * The previous hard caps (350 sidebar, 450 right) were too strict on wide
 * displays — users wanted to drag the right panel out to half the screen for
 * file review or terminal work. Cap scales with viewport so wider screens get
 * more room, while small screens still keep the center column usable.
 */

const MIN_CAP_PX = 800;
const VIEWPORT_RATIO = 0.7;
const FALLBACK_VIEWPORT = 1440;

/**
 * Compute the maximum pixel width for a pinned column at the current viewport.
 * Returns max(800, viewportWidth * 0.7). Falls back to 1440 when called in an
 * SSR/non-browser context.
 */
export function computePinnedMaxPx(viewportWidth?: number): number {
  const vw =
    viewportWidth ?? (typeof window !== "undefined" ? window.innerWidth : FALLBACK_VIEWPORT);
  return Math.max(MIN_CAP_PX, Math.round(vw * VIEWPORT_RATIO));
}

/** Minimum pixel width for any pinned column. Below this the panel becomes
 *  unusable (icons clipped, scrollbars stacked). */
export const LAYOUT_PINNED_MIN_PX = 180;
