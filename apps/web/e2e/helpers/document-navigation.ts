import type { Page } from "@playwright/test";

/**
 * Collect main-frame document navigation requests (reloads or full navigations)
 * observed after arming. A client-side route change does not issue one, so an
 * empty list proves the conversation appeared without recovering through a
 * document reload.
 *
 * Arm before the action whose handoff must stay client-side; the Playwright
 * `request` event only observes requests issued after the listener attaches.
 */
export function attachMainFrameNavigationCapture(page: Page): { requests: string[] } {
  const requests: string[] = [];
  page.on("request", (request) => {
    if (!request.isNavigationRequest()) return;
    if (request.frame() !== page.mainFrame()) return;
    requests.push(request.url());
  });
  return { requests };
}
