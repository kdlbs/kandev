import type { Locator, Page } from "@playwright/test";

export async function workspaceContextRetryRequested(page: Page): Promise<boolean> {
  return page.evaluate(
    () => document.documentElement.dataset.e2eWorkspaceContextRetry === "requested",
  );
}

/** Keep outage recovery tied to Retry even when a failed read replaces its button. */
export async function armWorkspaceContextRetry(surface: Locator): Promise<void> {
  await surface.evaluate((element) => {
    const allowRetry = (event: Event) => {
      const button = event.target instanceof Element ? event.target.closest("button") : null;
      if (
        button?.textContent?.trim() !== "Retry" ||
        !button.closest('[data-testid="sidebar-task-load-error"]')
      )
        return;
      document.documentElement.dataset.e2eWorkspaceContextRetry = "requested";
      element.removeEventListener("click", allowRetry, true);
    };
    element.addEventListener("click", allowRetry, true);
  });
}
