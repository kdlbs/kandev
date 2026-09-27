import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

const LATEST_MARKER = "JUMP_TO_LATEST_ASSISTANT_REPLY_4Q8K";
const STREAM_START_MARKER = "JUMP_TO_LATEST_STREAM_START_6M2J";
const STREAM_END_MARKER = "JUMP_TO_LATEST_STREAM_END_9R3V";

function messageCommand(content: string): string {
  return `e2e:message(${JSON.stringify(content)})`;
}

function assistantReply(chat: Locator, marker: string): Locator {
  return chat
    .locator("[data-agent-message-body][data-message-id]")
    .filter({ hasText: marker })
    .first();
}

function overflowScript(): string {
  const messages = Array.from({ length: 30 }, (_, index) =>
    messageCommand(
      `Jump history ${index + 1}: lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua`,
    ),
  );
  messages.push(messageCommand(LATEST_MARKER));
  return messages.join("\n");
}

async function seedOverflowingTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Jump to latest navigation",
    seedData.agentProfileId,
    {
      description: overflowScript(),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  await waitForSessionDone(apiClient, task.id, task.session_id, "latest history should finish");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

async function scrollUp(testPage: Page, list: ReturnType<SessionPage["activeChat"]>) {
  const startTop = await list.evaluate((element) => element.scrollTop);
  await list.hover();
  await testPage.mouse.wheel(0, -80);
  await expect
    .poll(async () => startTop - (await list.evaluate((element) => element.scrollTop)))
    .toBeGreaterThan(5);
}

test("Jump to latest returns to the newest grouped reply and preserves auto-scroll", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const session = await seedOverflowingTask(testPage, apiClient, seedData);
  const chat = session.activeChat();
  const list = chat.locator(".chat-message-list");
  const statusBar = chat.getByTestId("chat-status-bar");
  const button = statusBar.getByTestId("jump-to-latest-button");
  const autoScrollToggle = statusBar.getByTestId("auto-scroll-toggle-button");

  await expect
    .poll(async () =>
      list.evaluate((element) => element.scrollHeight - element.scrollTop - element.clientHeight),
    )
    .toBeLessThan(10);
  await expect(button).toHaveCount(0);

  await scrollUp(testPage, list);
  await expect(button).toBeVisible();
  await button.focus();
  await testPage.keyboard.press("Enter");

  await expect
    .poll(async () =>
      list.evaluate((element) => element.scrollHeight - element.scrollTop - element.clientHeight),
    )
    .toBeLessThan(10);
  await expect(assistantReply(chat, LATEST_MARKER)).toBeInViewport();
  await expect(button).toHaveCount(0);
  await expect(autoScrollToggle).toHaveAttribute("aria-pressed", "true");
  await expect
    .poll(async () => list.evaluate((element) => document.activeElement === element))
    .toBe(true);

  await scrollUp(testPage, list);
  await expect(button).toBeVisible();
  await expect(autoScrollToggle).toHaveAttribute("aria-pressed", "true");
  await autoScrollToggle.click();
  await expect(autoScrollToggle).toHaveAttribute("aria-pressed", "false");
  await button.click();
  await expect
    .poll(async () =>
      list.evaluate((element) => element.scrollHeight - element.scrollTop - element.clientHeight),
    )
    .toBeLessThan(10);
  await expect(button).toHaveCount(0);
  await expect(autoScrollToggle).toHaveAttribute("aria-pressed", "false");

  await session.sendMessageViaButton(
    `${messageCommand(STREAM_START_MARKER)}\ne2e:delay(1600)\n${messageCommand(STREAM_END_MARKER)}`,
  );
  await expect(assistantReply(chat, STREAM_START_MARKER)).toBeVisible();
  await scrollUp(testPage, list);
  await expect(button).toBeVisible();
  const readerTop = await list.evaluate((element) => element.scrollTop);
  await expect(assistantReply(chat, STREAM_END_MARKER)).toBeVisible({
    timeout: 10_000,
  });
  await expect
    .poll(async () => Math.abs((await list.evaluate((element) => element.scrollTop)) - readerTop))
    .toBeLessThanOrEqual(3);
  await expect(button).toBeVisible();

  await button.click();
  await expect(assistantReply(chat, STREAM_END_MARKER)).toBeInViewport();
  await expect(button).toHaveCount(0);
  await expect(autoScrollToggle).toHaveAttribute("aria-pressed", "false");
});
