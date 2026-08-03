import { expect, type Locator } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { expectFullQueueScrolls, seedFullQueueTask } from "./message-queue-scroll-helpers";

async function expectTouchTarget(locator: Locator): Promise<void> {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeGreaterThanOrEqual(44);
  expect(box!.height).toBeGreaterThanOrEqual(44);
}

test("mobile full queue stays usable while removing and clearing messages", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const session = await seedFullQueueTask(testPage, apiClient, seedData, "Mobile queue management");

  await expectFullQueueScrolls(session);

  const chat = session.activeChat();
  const panel = chat.getByTestId("queued-ghost-list");
  const entries = panel.getByTestId("queue-entry-text");
  const remove = panel.getByTestId("queue-entry-remove").nth(4);
  const clear = panel.getByTestId("queue-clear-all");
  const close = panel.getByTestId("queue-close");

  await remove.scrollIntoViewIfNeeded();
  await expectTouchTarget(remove);
  await expectTouchTarget(clear);
  await expectTouchTarget(close);

  await remove.tap();
  await expect(entries).toHaveCount(9, { timeout: 10_000 });
  await expect(panel).toContainText("9 of 10");
  await expect(panel.locator('[aria-label^="Position #"]').nth(4)).toHaveAttribute(
    "aria-label",
    "Position #5",
  );
  await expect(panel.locator('[aria-label^="Position #"]').last()).toHaveAttribute(
    "aria-label",
    "Position #9",
  );
  await expect(chat.getByTestId("chat-input-editor-shell")).toBeVisible();

  await clear.tap();
  await expect(panel).not.toBeVisible({ timeout: 10_000 });
  await expect(chat.getByTestId("queue-chip")).not.toBeVisible();
  await expect(chat.getByTestId("chat-input-editor-shell")).toBeVisible();
});
