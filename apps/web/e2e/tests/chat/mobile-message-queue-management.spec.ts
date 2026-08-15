import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { typeWhileBusy, waitForComposerQueueMode } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";
import { expectFullQueueScrolls, seedFullQueueTask } from "./message-queue-scroll-helpers";
import { registerSeparateQueueRows } from "../../helpers/message-queue-settings";

registerSeparateQueueRows(test);

async function expectTouchTarget(locator: Locator): Promise<void> {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeGreaterThanOrEqual(44);
  expect(box!.height).toBeGreaterThanOrEqual(44);
}

async function seedBusyQueueTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{ session: SessionPage; taskId: string }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile queue Send Now",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await session.sendMessageViaButton("/slow 30s");
  await session.agentStatus().waitFor({ state: "visible", timeout: 15_000 });
  await waitForComposerQueueMode(testPage);
  return { session, taskId: task.id };
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

test("mobile Send Now replaces a busy turn without hover or horizontal overflow", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);

  const { session } = await seedBusyQueueTask(testPage, apiClient, seedData);
  const chat = session.activeChat();
  const editor = chat.locator(".tiptap.ProseMirror:visible").first();
  const submit = testPage.getByTestId("submit-message-button");
  for (const message of ["mobile first", "/slow 10s mobile second", "mobile third"]) {
    await typeWhileBusy(testPage, editor, message);
    await expect(submit).toBeEnabled();
    await submit.tap();
  }

  await chat.getByTestId("queue-chip").tap();
  const panel = chat.getByTestId("queued-ghost-list");
  await expect(panel.getByTestId("queue-entry-text")).toHaveCount(3);
  const rowSendNow = panel.getByTestId("queue-entry-send-now").nth(1);
  const headerSendNow = panel.getByTestId("queue-send-now");
  await expectTouchTarget(rowSendNow);
  await expectTouchTarget(headerSendNow);

  const viewport = await testPage.evaluate(() => ({
    clientWidth: document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
  }));
  expect(viewport.scrollWidth).toBeLessThanOrEqual(viewport.clientWidth);

  await rowSendNow.tap();
  const transcript = chat.locator(".chat-message-list:visible");
  await expect(transcript.getByText("/slow 10s mobile second", { exact: true })).toBeVisible({
    timeout: 20_000,
  });
  await expect(panel.getByTestId("queue-entry-text")).toHaveCount(2, { timeout: 10_000 });
  await expect(panel.getByTestId("queue-entry-text").nth(0)).toContainText("mobile first");
  await expect(panel.getByTestId("queue-entry-text").nth(1)).toContainText("mobile third");
  await expect(session.chat).not.toContainText("Turn cancelled by user");
});

test("mobile queue panel hides the desktop-only pin and keeps its controls", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);

  const { session } = await seedBusyQueueTask(testPage, apiClient, seedData);
  const chat = session.activeChat();
  const editor = chat.locator(".tiptap.ProseMirror:visible").first();
  const submit = testPage.getByTestId("submit-message-button");
  await typeWhileBusy(testPage, editor, "mobile queued message");
  await expect(submit).toBeEnabled();
  await submit.tap();

  await chat.getByTestId("queue-chip").tap();
  const panel = chat.getByTestId("queued-ghost-list");
  await expect(panel).toBeVisible({ timeout: 10_000 });

  // The pin is a desktop-only control: it must not render on the mobile
  // queue panel, while the other header controls stay touch-sized.
  await expect(panel.getByTestId("queue-pin")).toHaveCount(0);
  await expectTouchTarget(panel.getByTestId("queue-clear-all"));
  await expectTouchTarget(panel.getByTestId("queue-close"));
});
