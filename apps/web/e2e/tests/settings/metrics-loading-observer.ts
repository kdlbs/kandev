import type { Page } from "@playwright/test";

type MetricsObserverWindow = Window & { __metricsUnavailableSeen?: boolean };

export async function observeMetricsUnavailable(page: Page) {
  await page.addInitScript(() => {
    const observedWindow = window as MetricsObserverWindow;
    observedWindow.__metricsUnavailableSeen = false;
    const observer = new MutationObserver((mutations) => {
      if (
        mutations.some((mutation) =>
          Array.from(mutation.addedNodes).some((node) =>
            node.textContent?.includes("Metrics unavailable"),
          ),
        )
      ) {
        observedWindow.__metricsUnavailableSeen = true;
      }
    });
    observer.observe(document, { childList: true, subtree: true });
  });
}

export async function metricsUnavailableWasRendered(page: Page) {
  return page.evaluate(() => (window as MetricsObserverWindow).__metricsUnavailableSeen ?? false);
}
