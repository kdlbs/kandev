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
