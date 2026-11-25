import { expect, type Locator, type Page } from "@playwright/test";

export async function attachCommentFile(
  page: Page,
  form: Page | Locator,
  name: string,
  content = "Evidence for " + name,
) {
  const upload = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" && response.url().endsWith("/api/v1/attachments"),
  );
  await form.getByTestId("comment-attachment-input").setInputFiles({
    name,
    mimeType: "text/plain",
    buffer: Buffer.from(content),
  });
  const response = await upload;
  expect(response.ok()).toBe(true);
  const descriptor = await response.json();
  expect(descriptor).toMatchObject({ name, delivery_mode: "path" });
  return descriptor.attachment_id as string;
}
