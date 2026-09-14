import fs from "node:fs";
import path from "node:path";
import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { seedRunningGeneratingSession } from "../../helpers/generating-session";
import {
  registerSeparateQueueRows,
  requestMessageQueueSettings,
} from "../../helpers/message-queue-settings";
import { routeSessionEntryRecovery } from "../../helpers/session-entry-recovery";
import { typeWhileBusy, waitForComposerQueueMode } from "../../helpers/type-while-busy";
import { routeMainWebSocketWithQueueAdmissionDrops } from "../../helpers/ws-drop";
import { openQuickChatWithAgent, sendQuickChatMessage } from "./quick-chat-helpers";
import { SessionPage } from "../../pages/session-page";

registerSeparateQueueRows(test);

const QUEUE_CAPACITY = 10;
const ATTACHMENT_NAME = "queued-admission-evidence.txt";

async function fillQueue(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  count = QUEUE_CAPACITY,
): Promise<Awaited<ReturnType<ApiClient["getQueueSessionIdentity"]>>> {
  const identity = await apiClient.getQueueSessionIdentity(taskId, sessionId);
  for (let index = 0; index < count; index += 1) {
    await apiClient.queueMessage(identity, `Preloaded queue item ${index + 1}`);
  }
  await expect
    .poll(async () => (await apiClient.getQueueStatus(identity)).count, {
      timeout: 15_000,
      message: "the queue should contain the preloaded entries",
    })
    .toBe(count);
  return identity;
}

async function queueFromTaskComposer(
  page: Parameters<typeof typeWhileBusy>[0],
  session: SessionPage,
  content: string,
): Promise<void> {
  const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");
  await typeWhileBusy(page, editor, content);
  await session.clickSubmitWhenReady();
}

test.describe("queue admission reliability", () => {
  test.describe.configure({ retries: 1 });

  test("clears an attached Task draft after a six second admission response", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const recovery = await routeSessionEntryRecovery(testPage);
    const { session, taskId, sessionId } = await seedRunningGeneratingSession(
      testPage,
      apiClient,
      seedData,
      "Queue admission attachment recovery",
      { sleepSeconds: 30 },
    );

    fs.mkdirSync(testInfo.outputDir, { recursive: true });
    const attachmentPath = path.join(testInfo.outputDir, ATTACHMENT_NAME);
    fs.writeFileSync(attachmentPath, "queue admission attachment bytes");
    await session.activeChat().locator('input[type="file"]').setInputFiles(attachmentPath);
    await expect(testPage.getByText(ATTACHMENT_NAME, { exact: true })).toBeVisible();

    recovery.delayNextResponses(
      "message.queue.add",
      1,
      6_000,
      "six second queue admission response",
    );
    await queueFromTaskComposer(testPage, session, "queue the attached follow-up");

    const identity = await apiClient.getQueueSessionIdentity(taskId, sessionId);
    await expect
      .poll(async () => (await apiClient.getQueueStatus(identity)).count, { timeout: 20_000 })
      .toBe(1);
    await expect
      .poll(() => recovery.delayedResponseCount("message.queue.add"), { timeout: 20_000 })
      .toBe(1);

    await expect(session.activeChat().locator(".tiptap.ProseMirror:visible")).toHaveText("", {
      timeout: 30_000,
    });
    await session.activeChat().getByTestId("queue-chip").click();
    const row = session.activeChat().getByTestId("queue-entry").first();
    await expect(row).toContainText("queue the attached follow-up");
    await expect(row).toContainText(ATTACHMENT_NAME);
  });

  test("reconciles a lost accepted Quick Chat response without duplicating it", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);
    const drops = await routeMainWebSocketWithQueueAdmissionDrops(testPage);
    const dialog = await openQuickChatWithAgent(testPage);
    await sendQuickChatMessage(dialog, testPage, "/sleep 30");
    await expect(testPage.getByRole("status", { name: /Agent is (starting|running)/ })).toBeVisible(
      {
        timeout: 15_000,
      },
    );
    await waitForComposerQueueMode(dialog);

    const editor = dialog.locator(".tiptap.ProseMirror:visible");
    const prompt = "recover the accepted Quick Chat admission";
    drops.dropNextQueueAddResponse();
    await typeWhileBusy(testPage, editor, prompt);
    await dialog.getByTestId("submit-message-button").click();

    await expect(editor).toHaveText("", { timeout: 30_000 });
    await expect(dialog.getByTestId("queue-chip")).toBeVisible({ timeout: 15_000 });
    await expect.poll(() => drops.queueAddRequestCount()).toBe(1);
    await expect.poll(() => drops.droppedResponseCount()).toBe(1);
  });

  test("keeps the Task draft when admission and reconciliation remain uncertain", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const drops = await routeMainWebSocketWithQueueAdmissionDrops(testPage);
    const { session } = await seedRunningGeneratingSession(
      testPage,
      apiClient,
      seedData,
      "Queue admission final uncertainty",
      { sleepSeconds: 60 },
    );
    await waitForComposerQueueMode(session.activeChat());

    const prompt = "keep this prompt after final uncertainty";
    drops.dropNextQueueAddResponse(2);
    drops.dropQueueAdmissionReconciliation();
    await queueFromTaskComposer(testPage, session, prompt);

    const toast = testPage
      .getByTestId("toast-message")
      .filter({ hasText: "Message send status unknown" });
    await expect(toast).toContainText("The connection dropped or timed out", { timeout: 60_000 });
    await expect(session.activeChat().locator(".tiptap.ProseMirror:visible")).toHaveText(prompt);
    await expect.poll(() => drops.queueAddRequestCount()).toBe(2);
    await expect.poll(() => drops.droppedResponseCount()).toBe(2);
  });

  test("rejects a full queue and then folds the retained draft when Auto-merge is enabled", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { session, taskId, sessionId } = await seedRunningGeneratingSession(
      testPage,
      apiClient,
      seedData,
      "Queue admission Auto-merge capacity",
      { sleepSeconds: 60 },
    );
    const identity = await fillQueue(apiClient, taskId, sessionId);
    await waitForComposerQueueMode(session.activeChat());

    const prompt = "fold this retained full queue draft";
    await queueFromTaskComposer(testPage, session, prompt);
    const fullToast = testPage.getByTestId("toast-message").filter({ hasText: "Message not sent" });
    await expect(fullToast).toContainText("The message queue is full", { timeout: 15_000 });
    await expect(session.activeChat().locator(".tiptap.ProseMirror:visible")).toHaveText(prompt);

    await requestMessageQueueSettings(apiClient, "PATCH", { auto_merge_enabled: true });
    await session.clickSubmitWhenReady();
    await expect(session.activeChat().locator(".tiptap.ProseMirror:visible")).toHaveText("");
    await expect
      .poll(async () => (await apiClient.getQueueStatus(identity)).count, { timeout: 20_000 })
      .toBe(QUEUE_CAPACITY);

    await session.activeChat().getByTestId("queue-chip").click();
    await expect(session.activeChat().getByTestId("queue-entry").last()).toContainText(prompt);
  });
});
