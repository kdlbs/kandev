import { expect, type Page } from "@playwright/test";

export async function writeSidebarFlag(
  page: Page,
  taskId: string,
  note: string,
  submit: () => Promise<void>,
): Promise<void> {
  const [response] = await Promise.all([
    page.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" &&
        new URL(response.url()).pathname === `/api/v1/tasks/${taskId}` &&
        response.request().postDataJSON()?.flag_note === note,
    ),
    submit(),
  ]);
  expect(response.ok()).toBe(true);
}

export async function failNextSidebarFlagWrite(page: Page, taskId: string): Promise<void> {
  const path = `**/api/v1/tasks/${taskId}`;
  await page.route(path, async function failFlag(route) {
    const request = route.request();
    if (request.method() !== "PATCH" || typeof request.postDataJSON()?.flag_note !== "string") {
      return route.continue();
    }
    await route.fulfill({ status: 500, json: { error: "Flag write failed" } });
    await page.unroute(path, failFlag);
  });
}
