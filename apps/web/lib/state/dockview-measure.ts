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
  if (typeof document === "undefined") {
    return { width: api.width, height: api.height };
  }
  const dv = document.querySelector(".dv-dockview") as HTMLElement | null;
  const parent = dv?.parentElement;
  if (!parent || parent.clientWidth <= 0 || parent.clientHeight <= 0) {
    return { width: api.width, height: api.height };
  }
  return { width: parent.clientWidth, height: parent.clientHeight };
}
