import fs from "node:fs";
import path from "node:path";
import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForAgentMessage, waitForSessionDone } from "../../helpers/session";
import { waitForComposerQueueMode } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";
import {
  MESSAGE_PREVIEW_MAX_CODE_UNITS,
  MESSAGE_PREVIEW_MAX_LINES,
} from "../../../lib/utils/message-preview";

const INCIDENT_LOG_LINE_COUNT = 3_921;

test.beforeEach(async ({ backend }) => {
  // This test asserts the queue UI. Pin the feature off so a steering-enabled
  // shard cannot change the input mode while the test is running.
  await backend.restart({ KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING: "false" });
});

test.afterEach(async ({ backend }) => {
  // Restore the worker baseline for the next test in the shard.
  await backend.restart();
});

function oversizedMessage(prefix: string): { source: string; tail: string; firstLine: string } {
  const tail = `${prefix}-TAIL-MARKER`;
  const logLine = (index: number) =>
    `2026-09-18T06:32:25Z INFO worker=${index} operation completed; duration=125ms status=ok`;
  const lines = Array.from({ length: INCIDENT_LOG_LINE_COUNT }, (_, index) => {
    if (index === INCIDENT_LOG_LINE_COUNT - 1) return tail;
    if (index === 0) return `${prefix} ${logLine(index)}`;
    return logLine(index);
  });
  return { source: lines.join("\n"), tail, firstLine: lines[0] };
}

async function expectBounded(scope: Locator, tail: string): Promise<Locator> {
  const preview = scope.getByTestId("bounded-message-preview");
  await expect(preview).toBeVisible();
  await expect(preview.getByTestId("bounded-message-preview-notice")).toBeVisible();
  expect(await preview.locator("br").count()).toBeLessThanOrEqual(MESSAGE_PREVIEW_MAX_LINES - 1);
  expect((await preview.textContent()) ?? "").not.toContain(tail);
  expect(((await preview.textContent()) ?? "").length).toBeLessThanOrEqual(
    MESSAGE_PREVIEW_MAX_CODE_UNITS + 200,
  );
  return preview;
}

async function expectTextDownload(
  page: Page,
  control: Locator,
  expected: string,
  fileName: string,
  targetPath: string,
): Promise<void> {
  const downloadPromise = page.waitForEvent("download");
  await control.tap();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe(fileName);
  await download.saveAs(targetPath);
  expect(fs.readFileSync(targetPath, "utf8")).toBe(expected);
}

async function createTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

async function switchMobileTask(testPage: Page, title: string) {
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  const sheet = testPage.getByRole("dialog", { name: "Tasks" });
  const taskRow = sheet.getByTestId("sidebar-task-item").filter({ hasText: title });
  await expect(taskRow).toBeVisible({ timeout: 15_000 });
  await taskRow.tap();
  await expect(sheet).not.toBeVisible({ timeout: 10_000 });
}

test("mobile oversized previews stay bounded, downloadable, and touch-sized", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(180_000);
  const { source, tail, firstLine } = oversizedMessage("MOBILE-OVERSIZED");
  const firstTitle = `Mobile oversized message ${Date.now()}`;
  const task = await createTask(apiClient, seedData, firstTitle);
  if (!task.session_id) throw new Error("oversized mobile task has no session_id");

  await testPage.goto(`/t/${task.id}`);
  let session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await session.sendMessageViaButton(source);
  await session.waitForChatIdle({ timeout: 60_000 });

  let chat = session.activeChat();
  const userBubble = chat.getByTestId("user-message-bubble").filter({ hasText: firstLine }).first();
  await expect(userBubble).toBeVisible();
  await expectBounded(userBubble, tail);
  const userDownload = userBubble.getByRole("button", { name: "Download full text" });
  const userDownloadBox = await userDownload.boundingBox();
  expect(userDownloadBox?.height).toBeGreaterThanOrEqual(44);
  expect(userDownloadBox?.width).toBeGreaterThanOrEqual(44);
  await expectTextDownload(
    testPage,
    userDownload,
    source,
    "kandev-message.txt",
    path.join(backend.tmpDir, `mobile-message-${Date.now()}.txt`),
  );
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(task.session_id!);
        return messages.some(
          (message) => message.author_type === "user" && message.content === source,
        );
      },
      { timeout: 30_000, message: "the complete mobile oversized prompt should be stored" },
    )
    .toBe(true);

  await testPage.reload();
  session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  chat = session.activeChat();
  await expect(chat.getByTestId("user-message-bubble").filter({ hasText: firstLine })).toHaveCount(
    1,
  );
  await assertNoDocumentHorizontalOverflow(testPage, "mobile oversized message flow");

  const secondTitle = `Mobile switch target ${Date.now()}`;
  const secondTask = await createTask(apiClient, seedData, secondTitle);
  await switchMobileTask(testPage, secondTitle);
  await expect(testPage).toHaveURL(new RegExp(`/t/${secondTask.id}`));
  session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await switchMobileTask(testPage, firstTitle);
  await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));
  session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  chat = session.activeChat();
  await expect(chat.getByTestId("user-message-bubble").filter({ hasText: firstLine })).toHaveCount(
    1,
  );

  const slowPrompt = "/slow 30s";
  await session.sendMessageViaButton(slowPrompt);
  await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
  await waitForComposerQueueMode(testPage);
  const queued = oversizedMessage("MOBILE-QUEUED");
  const identity = await apiClient.getQueueSessionIdentity(task.id, task.session_id);
  // Keep the preview available even if the agent finishes during the touch checks.
  await apiClient.setQueueAutoRun(identity, false);
  await apiClient.queueMessage(identity, queued.source);
  await expect(chat.getByTestId("queue-chip")).toBeVisible({ timeout: 15_000 });
  await chat.getByTestId("queue-chip").tap();
  const panel = chat.getByTestId("queued-ghost-list");
  await expect(panel).toBeVisible();
  const row = panel.getByTestId("queue-entry");
  await expect(row).toBeVisible();
  await expectBounded(row, queued.tail);

  const expand = row.getByTestId("queue-entry-expand");
  const expandBox = await expand.boundingBox();
  expect(expandBox?.height).toBeGreaterThanOrEqual(44);
  expect(expandBox?.width).toBeGreaterThanOrEqual(44);
  await expand.tap();
  await expect(expand).toHaveAttribute("aria-expanded", "true");
  await expectBounded(row, queued.tail);

  const queuedDownload = row.getByRole("button", { name: "Download full text" });
  const queuedDownloadBox = await queuedDownload.boundingBox();
  expect(queuedDownloadBox?.height).toBeGreaterThanOrEqual(44);
  expect(queuedDownloadBox?.width).toBeGreaterThanOrEqual(44);
  await expectTextDownload(
    testPage,
    queuedDownload,
    queued.source,
    "kandev-queued-message.txt",
    path.join(backend.tmpDir, `mobile-queued-${Date.now()}.txt`),
  );
  await apiClient.clearQueue(identity);
  await expect(chat.getByTestId("queue-chip")).not.toBeVisible({ timeout: 15_000 });
  await expect(chat.getByText("Slow response complete", { exact: false })).toBeVisible({
    timeout: 60_000,
  });
  await session.waitForChatIdle({ timeout: 60_000 });
  await waitForAgentMessage(apiClient, task.session_id, "Slow response complete", 30_000);
  await waitForSessionDone(
    apiClient,
    task.id,
    task.session_id,
    "the mobile follow-up session should finish",
    30_000,
  );
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(task.session_id!);
        return messages.some(
          (message) => message.author_type === "user" && message.content === slowPrompt,
        );
      },
      { timeout: 30_000, message: "the mobile follow-up prompt should be stored" },
    )
    .toBe(true);
  await assertNoDocumentHorizontalOverflow(testPage, "mobile queued oversized message flow");
});
