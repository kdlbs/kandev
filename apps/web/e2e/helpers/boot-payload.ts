import type { Page } from "@playwright/test";

export async function rewriteBackendHostOS(page: Page, hostOS: string): Promise<void> {
  await page.route("**/*", async (route) => {
    if (route.request().resourceType() !== "document") {
      await route.continue();
      return;
    }
    const response = await route.fetch();
    const body = await response.text();
    const rewritten = body.replace(/"hostOS":"[^"]*"/, `"hostOS":"${hostOS}"`);
    if (rewritten === body) {
      throw new Error("The boot document did not contain runtime.hostOS");
    }
    await route.fulfill({ response, body: rewritten });
  });
}
