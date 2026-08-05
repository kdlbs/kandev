export const TASK_ROW_DOM_ATTR = "data-task-row-id";
export const TASK_SIDEBAR_SCROLL_SELECTOR = '[data-testid="task-sidebar-scroll"]';

const MAX_TASK_NAVIGATION_ATTEMPTS = 60;
let latestNavigationRequestId = 0;

/** Invalidates the current reveal so a pending selection cannot scroll a stale row. */
export function cancelSidebarTaskReveal(): void {
  latestNavigationRequestId += 1;
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
    rowRect.top >= viewportRect.top &&
    rowRect.bottom <= viewportRect.bottom &&
    rowRect.left >= viewportRect.left &&
    rowRect.right <= viewportRect.right
  );
}

function defaultRequestFrame(callback: () => void): void {
  if (typeof requestAnimationFrame === "function") {
    requestAnimationFrame(callback);
  } else {
    setTimeout(callback, 16);
  }
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
    const tick = () => {
      if (requestId !== latestNavigationRequestId) {
        resolve(false);
        return;
      }

      const match = findVisibleTaskRow(taskId);
      if (match) {
        if (requestId !== latestNavigationRequestId) {
          resolve(false);
          return;
        }
        if (!isInsideViewport(match.row, match.viewport)) {
          match.row.scrollIntoView({ block: "nearest", inline: "nearest" });
        }
        resolve(true);
        return;
      }

      attempts += 1;
      if (attempts >= MAX_TASK_NAVIGATION_ATTEMPTS) {
        resolve(false);
        return;
      }
      requestFrame(tick);
    };
    tick();
  });
}
