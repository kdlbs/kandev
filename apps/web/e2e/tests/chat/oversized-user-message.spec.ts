import fs from "node:fs";
import path from "node:path";
import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { watchWs } from "../../helpers/causal-waits";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";
import { waitForComposerQueueMode } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";
import {
  MESSAGE_PREVIEW_MAX_LINES,
  MESSAGE_PREVIEW_MAX_CODE_UNITS,
} from "../../../lib/utils/message-preview";

const INCIDENT_LOG_LINE_COUNT = 3_921;

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
  await control.click();
  const download = await downloadPromise;
  expect(download.suggestedFilename()).toBe(fileName);
  await download.saveAs(targetPath);
  expect(fs.readFileSync(targetPath, "utf8")).toBe(expected);
}

async function createTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  workflowId = seedData.workflowId,
  workflowStepId = seedData.startStepId,
) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: workflowId,
    workflow_step_id: workflowStepId,
    repository_ids: [seedData.repositoryId],
  });
}

async function seedPinnedFiller(apiClient: ApiClient, sessionId: string): Promise<void> {
  const filler = Array.from({ length: 40 }, (_, index) => `pinned filler line ${index + 1}`).join(
    "\n",
  );
  for (let index = 0; index < 8; index++) {
    await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: `${filler}\nPinned filler message ${index + 1}`,
    });
  }
}

test.describe("Oversized user-message previews", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings({
      show_anchored_prompt_bar: false,
      show_scroll_to_last_prompt: true,
      show_scroll_to_start: false,
      show_transcript_auto_scroll_control: true,
    });
  });

  test("bounds transcript and pinned previews while preserving source and follow-up turns", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    const gateway = watchWs(testPage);
    const { source, tail, firstLine } = oversizedMessage("DESKTOP-OVERSIZED");
    await apiClient.saveUserSettings({
      show_anchored_prompt_bar: true,
      show_scroll_to_last_prompt: true,
      show_scroll_to_start: true,
    });

    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      `Oversized message follow-up ${Date.now()}`,
    );
    const startStep = await apiClient.createWorkflowStep(workflow.id, "Start", 0, {
      is_start_step: true,
      events: { on_turn_complete: [{ type: "move_to_next" }] },
    });
    await apiClient.createWorkflowStep(workflow.id, "Keep conversation open", 1, {
      complete_task_on_enter: false,
    });
    const firstTitle = `Desktop oversized message ${Date.now()}`;
    const task = await createTask(apiClient, seedData, firstTitle, workflow.id, startStep.id);
    if (!task.session_id) throw new Error("oversized desktop task has no session_id");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const editor = await session.fillMessage(source);
    await session.submitMessageWithKeyboard(editor);
    // The durable transcript confirms acceptance even if the large WS reply is
    // delayed after the user message has already rendered.
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(task.session_id!);
          return messages.some(
            (message) => message.author_type === "user" && message.content === source,
          );
        },
        { timeout: 30_000, message: "the complete oversized prompt should be stored" },
      )
      .toBe(true);
    await session.waitForChatIdle({ timeout: 60_000 });

    const chat = session.activeChat();
    const userBubble = chat
      .getByTestId("user-message-bubble")
      .filter({ hasText: firstLine })
      .first();
    await expect(userBubble).toBeVisible();
    await expectBounded(userBubble, tail);
    await expectTextDownload(
      testPage,
      userBubble.getByRole("button", { name: "Download full text" }),
      source,
      "kandev-message.txt",
      path.join(backend.tmpDir, `desktop-message-${Date.now()}.txt`),
    );

    await testPage.reload();
    await session.waitForLoad();
    await session.showSessionContext(30_000);
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(
      chat.getByTestId("user-message-bubble").filter({ hasText: firstLine }),
    ).toHaveCount(1);

    await seedPinnedFiller(apiClient, task.session_id);
    await expect(chat.getByText("Pinned filler message 8", { exact: false })).toBeVisible({
      timeout: 15_000,
    });
    await chat.locator(".chat-message-list").evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });

    const pinned = chat.getByTestId("anchored-last-prompt-bar");
    await expect(pinned).toHaveAttribute("data-state", "open", { timeout: 15_000 });
    await expectBounded(pinned, tail);
    const pinnedExpand = pinned.getByTestId("anchored-last-prompt-expand");
    await expect(pinnedExpand).toBeVisible();
    await pinnedExpand.click();
    await expect(pinnedExpand).toHaveAttribute("aria-expanded", "true");
    await expectBounded(pinned, tail);
    await expectTextDownload(
      testPage,
      pinned.getByRole("button", { name: "Download full text" }),
      source,
      "kandev-last-prompt.txt",
      path.join(backend.tmpDir, `desktop-pinned-${Date.now()}.txt`),
    );

    const secondTitle = `Desktop switch target ${Date.now()}`;
    const secondTask = await createTask(apiClient, seedData, secondTitle);
    await expect(session.sidebarTaskItem(secondTitle)).toBeVisible({ timeout: 15_000 });
    await session.sidebarTaskItem(secondTitle).click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${secondTask.id}`));
    await session.waitForLoad();
    await session.showSessionContext(30_000);
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(session.sidebarTaskItem(firstTitle)).toBeVisible({ timeout: 15_000 });
    await session.sidebarTaskItem(firstTitle).click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));
    await session.waitForLoad();
    await session.showSessionContext(30_000);
    await session.waitForChatIdle({ timeout: 30_000 });
    await expect(
      chat.getByTestId("user-message-bubble").filter({ hasText: firstLine }),
    ).toHaveCount(1);

    const followUp = "/e2e:multi-turn";
    const followUpAdded = gateway.waitForResponse("message.add");
    await session.sendMessage(followUp);
    await followUpAdded;
    await expect(chat.getByText(followUp, { exact: false }).last()).toBeVisible({
      timeout: 15_000,
    });
    await session.waitForChatIdle({ timeout: 60_000 });
    await session.expectChatResponseVisible("Multi-turn response ready", 0, { timeout: 30_000 });
    await waitForAgentMessage(apiClient, task.session_id, "Multi-turn response ready", 30_000);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "the desktop follow-up session should finish",
      timeout: 30_000,
    });
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(task.session_id!);
          return messages.some(
            (message) => message.author_type === "user" && message.content === followUp,
          );
        },
        { timeout: 30_000, message: "the follow-up prompt should be stored" },
      )
      .toBe(true);
    await assertNoDocumentHorizontalOverflow(testPage, "desktop oversized message flow");
  });

  test("keeps the download action touch-sized on a narrow fine-pointer viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 500, height: 900 });
    const source = Array.from(
      { length: MESSAGE_PREVIEW_MAX_LINES + 20 },
      (_, index) => `NARROW-FINE-POINTER-${index}`,
    ).join("\n");
    const firstLine = source.split("\n", 1)[0];
    const task = await createTask(apiClient, seedData, `Narrow fine pointer ${Date.now()}`);
    if (!task.session_id) throw new Error("narrow fine-pointer task has no session_id");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await session.sendMessageViaButton(source);
    await session.waitForChatIdle({ timeout: 60_000 });

    const userBubble = session
      .activeChat()
      .getByTestId("user-message-bubble")
      .filter({ hasText: firstLine })
      .first();
    await expect(userBubble).toBeVisible();
    const download = userBubble.getByRole("button", { name: "Download full text" });
    const downloadBox = await download.boundingBox();
    expect(downloadBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage, "narrow fine-pointer preview");
  });

  test("bounds queued previews and keeps their full source expandable", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const { source, tail } = oversizedMessage("DESKTOP-QUEUED");
    const task = await createTask(apiClient, seedData, `Desktop queued oversized ${Date.now()}`);
    if (!task.session_id) throw new Error("oversized queued task has no session_id");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    await session.sendMessage("/slow 30s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await waitForComposerQueueMode(testPage);
    const identity = await apiClient.getQueueSessionIdentity(task.id, task.session_id);
    await apiClient.queueMessage(identity, source);

    const chat = session.activeChat();
    await expect(chat.getByTestId("queue-chip")).toBeVisible({ timeout: 15_000 });
    await chat.getByTestId("queue-chip").click();
    const panel = chat.getByTestId("queued-ghost-list");
    await expect(panel).toBeVisible();
    const row = panel.getByTestId("queue-entry");
    await expect(row).toBeVisible();
    await row.hover();
    await expectBounded(row, tail);
    const expand = row.getByTestId("queue-entry-expand");
    await expect(expand).toBeVisible();
    await expand.click();
    await expect(expand).toHaveAttribute("aria-expanded", "true");
    await expectBounded(row, tail);
    await expectTextDownload(
      testPage,
      row.getByRole("button", { name: "Download full text" }),
      source,
      "kandev-queued-message.txt",
      path.join(backend.tmpDir, `desktop-queued-${Date.now()}.txt`),
    );
  });
});
