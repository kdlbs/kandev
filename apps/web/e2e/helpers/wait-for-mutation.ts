import type { Page } from "@playwright/test";

export function waitForMutation(page: Page, method: string, path: RegExp) {
  return page.waitForResponse((response) => {
    const request = response.request();
    return request.method() === method && path.test(new URL(response.url()).pathname);
  });
}
