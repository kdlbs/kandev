import { releasePortalScrollRestoration } from "@/lib/layout/panel-portal-host";

export const TASK_ROW_DOM_ATTR = "data-task-row-id";
export const TASK_SIDEBAR_SCROLL_SELECTOR = '[data-testid="task-sidebar-scroll"]';
export const TASK_ROW_REVEAL_CLASS = "task-sidebar-row-reveal";

const MAX_TASK_NAVIGATION_ATTEMPTS = 60;
const STABLE_VISIBLE_FRAME_COUNT = 3;
const TASK_ROW_REVEAL_DURATION_MS = 1400;
let latestNavigationRequestId = 0;
let latestCueId = 0;

type ActiveTaskRowCue = {
  cueId: number;
  row: HTMLElement;
  timeoutId: number;
};

let activeTaskRowCue: ActiveTaskRowCue | null = null;

/** Invalidates the current reveal so a pending selection cannot scroll a stale row. */
export function cancelSidebarTaskReveal(): void {
  latestNavigationRequestId += 1;
  if (!activeTaskRowCue) return;

  window.clearTimeout(activeTaskRowCue.timeoutId);
  activeTaskRowCue.row.classList.remove(TASK_ROW_REVEAL_CLASS);
  activeTaskRowCue = null;
}

/** CSS selector for a rendered task row by its stable task id. */
export function taskRowSelector(taskId: string): string {
  return `[${TASK_ROW_DOM_ATTR}="${CSS.escape(taskId)}"]`;
}

function isVisible(element: HTMLElement, stopBefore?: HTMLElement): boolean {
  for (
    let current: HTMLElement | null = element;
    current && current !== stopBefore;
    current = current.parentElement
  ) {
    const styles = window.getComputedStyle(current);
    if (
      styles.display === "none" ||
      styles.visibility === "hidden" ||
      styles.visibility === "collapse"
    ) {
      return false;
    }
  }
  const rect = element.getBoundingClientRect();
  return rect.width > 0 && rect.height > 0;
}

function findVisibleTaskRow(taskId: string): { row: HTMLElement; viewport: HTMLElement } | null {
  const selector = taskRowSelector(taskId);
  const viewports = document.querySelectorAll<HTMLElement>(TASK_SIDEBAR_SCROLL_SELECTOR);
  for (const viewport of viewports) {
    if (!isVisible(viewport)) continue;
    const row = viewport.querySelector<HTMLElement>(selector);
    if (row && isVisible(row, viewport)) return { row, viewport };
  }
  return null;
}

function isInsideViewport(row: HTMLElement, viewport: HTMLElement): boolean {
  const rowRect = row.getBoundingClientRect();
  const viewportRect = viewport.getBoundingClientRect();
  return (
    rowRect.top >= viewportRect.top - 1 &&
    rowRect.bottom <= viewportRect.bottom + 1 &&
    rowRect.left >= viewportRect.left - 1 &&
    rowRect.right <= viewportRect.right + 1
  );
}

function defaultRequestFrame(callback: () => void): void {
  if (typeof requestAnimationFrame === "function") {
    requestAnimationFrame(callback);
  } else {
    setTimeout(callback, 16);
  }
}

function prefersReducedMotion(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches
  );
}

/** Restarts the short-lived cue on the latest command-selected row. */
function cueTaskRow(row: HTMLElement): void {
  if (activeTaskRowCue) {
    window.clearTimeout(activeTaskRowCue.timeoutId);
    activeTaskRowCue.row.classList.remove(TASK_ROW_REVEAL_CLASS);
  }

  const cueId = ++latestCueId;
  row.classList.remove(TASK_ROW_REVEAL_CLASS);
  // Force a reflow so selecting the same task twice restarts its animation.
  void row.offsetWidth;
  row.classList.add(TASK_ROW_REVEAL_CLASS);

  const timeoutId = window.setTimeout(() => {
    if (activeTaskRowCue?.cueId !== cueId) return;
    row.classList.remove(TASK_ROW_REVEAL_CLASS);
    activeTaskRowCue = null;
  }, TASK_ROW_REVEAL_DURATION_MS);
  activeTaskRowCue = { cueId, row, timeoutId };
}

type TaskRowVisibilityState = {
  portalScrollRestoreReleased: boolean;
  scrollRequested: boolean;
  visibleFrames: number;
  previousGeometry: [number, number, number, number, number] | null;
};

function updateTaskRowVisibility(
  match: { row: HTMLElement; viewport: HTMLElement },
  state: TaskRowVisibilityState,
): boolean {
  if (!state.portalScrollRestoreReleased) {
    releasePortalScrollRestoration(match.viewport);
    state.portalScrollRestoreReleased = true;
  }
  if (!isInsideViewport(match.row, match.viewport)) {
    // A row displaced after entering view starts a new scroll; pending motion does not.
    if (state.previousGeometry !== null) state.scrollRequested = false;
    state.visibleFrames = 0;
    state.previousGeometry = null;
    if (!state.scrollRequested) {
      match.row.scrollIntoView({
        behavior: prefersReducedMotion() ? "auto" : "smooth",
        block: "nearest",
        inline: "nearest",
      });
      state.scrollRequested = true;
    }
    return false;
  }

  const rowRect = match.row.getBoundingClientRect();
  const viewportRect = match.viewport.getBoundingClientRect();
  const geometry: [number, number, number, number, number] = [
    rowRect.top,
    rowRect.bottom,
    viewportRect.top,
    viewportRect.bottom,
    match.viewport.scrollTop,
  ];
  const geometryStable = state.previousGeometry?.every(
    (value, index) => Math.abs(value - geometry[index]) <= 0.5,
  );
  state.visibleFrames = geometryStable ? state.visibleFrames + 1 : 1;
  state.previousGeometry = geometry;
  if (state.visibleFrames < STABLE_VISIBLE_FRAME_COUNT) return false;

  releasePortalScrollRestoration(match.viewport);
  cueTaskRow(match.row);
  return true;
}

/**
 * Reveals a rendered task row in the visible desktop sidebar.
 *
 * The row may mount after route navigation or sidebar hydration, so lookup is
 * retried for a bounded number of animation frames. A missing or hidden row is
 * intentionally a no-op so task navigation never depends on sidebar state.
 */
export function revealSidebarTask(
  taskId: string,
  requestFrame: (callback: () => void) => void = defaultRequestFrame,
): Promise<boolean> {
  if (typeof document === "undefined") return Promise.resolve(false);

  const requestId = ++latestNavigationRequestId;
  return new Promise((resolve) => {
    let attempts = 0;
    const visibilityState: TaskRowVisibilityState = {
      portalScrollRestoreReleased: false,
      scrollRequested: false,
      visibleFrames: 0,
      previousGeometry: null,
    };
    const tick = () => {
      if (requestId !== latestNavigationRequestId) {
        resolve(false);
        return;
      }

      const match = findVisibleTaskRow(taskId);
      if (match) {
        if (updateTaskRowVisibility(match, visibilityState)) {
          resolve(true);
          return;
        }
      } else {
        visibilityState.visibleFrames = 0;
        visibilityState.previousGeometry = null;
      }

      attempts += 1;
      if (attempts >= MAX_TASK_NAVIGATION_ATTEMPTS) {
        resolve(false);
        return;
      }
      requestFrame(tick);
    };
    requestFrame(tick);
  });
}
