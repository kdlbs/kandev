import type { DockviewApi } from "dockview-react";

/**
 * Measure the live size of the dockview container element.
 *
 * `api.width` / `api.height` reflect dockview's *internal* grid dimensions,
 * which can drift out of sync with the actual DOM container — e.g. after a
 * `fromJSON` whose recorded `grid.width` differs from the live container, or
 * after a window/devtools resize that the library's ResizeObserver missed.
 * Reading `clientWidth/Height` of `.dv-dockview`'s parent element is the
 * source of truth and lets us recover from a drifted internal width.
 */
export function measureDockviewContainer(api: DockviewApi): { width: number; height: number } {
  const live = liveContainerSize();
  if (live) return live;
  // No laid-out container (e.g. a fresh client-side navigation mounts dockview
  // before the grid has painted). `api.width/height` are 0 at that point, and
  // building the default layout at width 0 makes dockview collapse the
  // horizontal columns into a vertical stack (chat / files+changes / terminal).
  // Fall back to the viewport so the default builds horizontally; the resize
  // observer then snaps the grid to the exact container size.
  return {
    width: api.width > 0 ? api.width : viewportWidth(),
    height: api.height > 0 ? api.height : viewportHeight(),
  };
}

// The dockview content area sits next to the unified AppSidebar (#1165), whose
// width animates over 300ms (`transition-all`). During that animation `onReady`
// can read the parent at a tiny positive width — building the default layout at
// that width collapses the horizontal columns into a vertical stack. The
// sidebar maxes at 30vw (desktop-only surface), so a healthy content width is
// always >= ~70vw; anything below half the viewport is a mid-transition
// transient and must be treated like a not-yet-laid-out (0-width) container.
const MIN_PLAUSIBLE_WIDTH_RATIO = 0.5;

function liveContainerSize(): { width: number; height: number } | null {
  if (typeof document === "undefined") return null;
  const dv = document.querySelector(".dv-dockview") as HTMLElement | null;
  const parent = dv?.parentElement;
  if (!parent || parent.clientWidth <= 0 || parent.clientHeight <= 0) return null;
  // The sidebar animation is horizontal, so only the width can be a transient —
  // the live height is reliable. Fall back just the width to the viewport so the
  // build stays horizontal; keep the real height to avoid a needless vertical
  // transient. The resize observer then snaps the width to the exact size.
  const plausibleWidth = parent.clientWidth >= viewportWidth() * MIN_PLAUSIBLE_WIDTH_RATIO;
  return {
    width: plausibleWidth ? parent.clientWidth : viewportWidth(),
    height: parent.clientHeight,
  };
}

function viewportWidth(): number {
  return typeof window !== "undefined" && window.innerWidth > 0 ? window.innerWidth : 1280;
}

function viewportHeight(): number {
  return typeof window !== "undefined" && window.innerHeight > 0 ? window.innerHeight : 800;
}
