import type { Page } from "@playwright/test";

export async function holdPluginInstallResponse(page: Page) {
  let release = () => {};
  let requestStarted = false;
  let markResponseSettled = () => {};
  const responseHeld = new Promise<void>((resolve) => {
    release = resolve;
  });
  const responseSettled = new Promise<void>((resolve) => {
    markResponseSettled = resolve;
  });
  let markRequestSeen = () => {};
  const requestSeen = new Promise<void>((resolve) => {
    markRequestSeen = resolve;
  });

  await page.route("**/api/plugins/install", async (route) => {
    requestStarted = true;
    markRequestSeen();
    try {
      await responseHeld;
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "install failed" }),
      });
    } finally {
      markResponseSettled();
    }
  });

  return { requestSeen, responseSettled, release, requestStarted: () => requestStarted };
}
