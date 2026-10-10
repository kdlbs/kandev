import { type Locator, type Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export type QuickChatStartResponse = {
  session_id: string;
  task_id: string;
};

type OpeningMessageAttachment = {
  name?: string;
  attachment_id?: string;
};

type CompleteQuickChatOpeningOptions = {
  submit: (button: Locator) => Promise<void>;
  prepare?: () => Promise<void>;
  expectedAttachmentName?: string;
};

export async function completeQuickChatOpening(
  page: Page,
  dialog: Locator,
  apiClient: ApiClient,
  prompt: string,
  options: CompleteQuickChatOpeningOptions,
): Promise<QuickChatStartResponse> {
  const editor = dialog.getByTestId("task-description-input");
  const send = dialog.getByTestId("quick-chat-send");
  await expect(send).toBeDisabled();
  await options.prepare?.();
  await editor.fill(prompt);
  await expect(send).toBeEnabled();

  const startResponse = page.waitForResponse(
    (response) =>
      response.url().includes("/quick-chat") &&
      response.request().method() === "POST" &&
      response.ok(),
  );
  await options.submit(send);
  const started = (await (await startResponse).json()) as QuickChatStartResponse;
  expect(started.session_id).toBeTruthy();
  expect(started.task_id).toBeTruthy();

  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(started.session_id);
        return messages.filter(
          (message) => message.author_type === "user" && message.content.includes(prompt),
        ).length;
      },
      { timeout: 30_000, message: `opening prompt was not delivered to ${started.session_id}` },
    )
    .toBe(1);

  if (options.expectedAttachmentName) {
    const { messages } = await apiClient.listSessionMessages(started.session_id);
    const message = messages.find(
      (candidate) => candidate.author_type === "user" && candidate.content.includes(prompt),
    );
    const attachments = message?.metadata?.attachments;
    expect(Array.isArray(attachments)).toBe(true);
    expect((attachments as OpeningMessageAttachment[])[0]).toMatchObject({
      name: options.expectedAttachmentName,
    });
    expect((attachments as OpeningMessageAttachment[])[0]?.attachment_id).toBeTruthy();
  }

  await expect(
    dialog
      .getByTestId("quick-chat-messages")
      .getByTestId("user-message-bubble")
      .filter({ hasText: prompt }),
  ).toBeVisible({ timeout: 15_000 });
  await expect(dialog.locator(".tiptap.ProseMirror:visible").last()).toBeEmpty();
  return started;
}

export async function expectTouchTarget(control: Locator, minimum = 44): Promise<void> {
  const box = await control.boundingBox();
  expect(box).not.toBeNull();
  if (!box) return;
  expect(box.width).toBeGreaterThanOrEqual(minimum);
  expect(box.height).toBeGreaterThanOrEqual(minimum);
}

export async function expectNoHorizontalOverflow(page: Page): Promise<void> {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
}

export async function expectContextRowSpacing(dialog: Locator, chip: Locator): Promise<void> {
  const padding = await chip.evaluate((element) => {
    let parent = element.parentElement;
    while (parent && getComputedStyle(parent).display === "contents") {
      parent = parent.parentElement;
    }
    if (!parent) throw new Error("context row missing");
    const style = getComputedStyle(parent);
    return { top: parseFloat(style.paddingTop), bottom: parseFloat(style.paddingBottom) };
  });
  expect(padding).toEqual({ top: 8, bottom: 8 });
  const row = dialog.getByTestId("composer-context-row");
  await expect(row).toHaveCount(1);
  const rowBox = await row.boundingBox();
  const chipBox = await chip.boundingBox();
  if (!rowBox || !chipBox) throw new Error("context row bounds missing");
  expect(
    Math.abs(chipBox.y - rowBox.y - (rowBox.y + rowBox.height - chipBox.y - chipBox.height)),
  ).toBeLessThanOrEqual(1);
}

export async function expectMixedContextRow(dialog: Locator, sameLine: boolean): Promise<void> {
  const buffer = Buffer.from(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j7xkAAAAASUVORK5CYII=",
    "base64",
  );
  await dialog.locator('input[type="file"]').setInputFiles([
    { name: "context-one.png", mimeType: "image/png", buffer },
    { name: "context-two.png", mimeType: "image/png", buffer },
  ]);
  const row = dialog.getByTestId("composer-context-row");
  const images = row.getByTestId("context-chip-remove").locator("xpath=..");
  await expect(images).toHaveCount(2);
  await expect(row.getByTestId("repo-chip")).toHaveCount(1);
  expect(
    await images.evaluateAll((elements) =>
      elements.every((element) => element.parentElement?.dataset.testid === "composer-context-row"),
    ),
  ).toBe(true);
  expect(await row.evaluate((element) => getComputedStyle(element).display)).toBe("flex");
  if (sameLine) {
    const repoBox = await row.getByTestId("repo-chip").boundingBox();
    const imageBox = await images.nth(1).boundingBox();
    if (!repoBox || !imageBox) throw new Error("mixed context bounds missing");
    expect(imageBox.x).toBeGreaterThan(repoBox.x + repoBox.width);
    expect(
      Math.abs(repoBox.y + repoBox.height / 2 - imageBox.y - imageBox.height / 2),
    ).toBeLessThanOrEqual(1);
  }
}
