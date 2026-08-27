import { expect, type Locator, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { typeWhileBusy, waitForComposerQueueMode } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";
import { expectFullQueueScrolls, seedFullQueueTask } from "./message-queue-scroll-helpers";
import { registerSeparateQueueRows } from "../../helpers/message-queue-settings";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

registerSeparateQueueRows(test);

async function expectTouchTarget(locator: Locator): Promise<void> {
  await expect(locator).toBeVisible();
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.width).toBeGreaterThanOrEqual(44);
  expect(box!.height).toBeGreaterThanOrEqual(44);
}

async function expectEffectiveTouchTarget(locator: Locator): Promise<void> {
  await expect(locator).toBeVisible();
  const size = await locator.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    const after = getComputedStyle(element, "::after");
    const px = (value: string) => Number.parseFloat(value) || 0;
    return {
      width: rect.width - px(after.left) - px(after.right),
      height: rect.height - px(after.top) - px(after.bottom),
    };
  });
  expect(size.width).toBeGreaterThanOrEqual(44);
  expect(size.height).toBeGreaterThanOrEqual(44);
}

function scriptedQueueMessage(marker: string, delayMs = 250): string {
  return `e2e:delay(${delayMs})\ne2e:message("${marker}")`;
}

async function expectSeparateTurnsInOrder(scope: Locator, markers: string[]): Promise<void> {
  const agentBodies = scope.locator("[data-agent-message-body][data-message-id]");
  for (const marker of markers) {
    await expect(agentBodies.filter({ hasText: marker })).toHaveCount(1, { timeout: 45_000 });
  }
  const agentTexts = await agentBodies.allTextContents();
  const agentIndexes = markers.map((marker) =>
    agentTexts.findIndex((text) => text.includes(marker)),
  );
  expect(agentIndexes).toEqual([...agentIndexes].sort((a, b) => a - b));

  const userTexts = await scope.getByTestId("user-message-bubble").allTextContents();
  const userIndexes = markers.map((marker) => userTexts.findIndex((text) => text.includes(marker)));
  expect(userIndexes.every((index) => index >= 0)).toBe(true);
  expect(new Set(userIndexes).size).toBe(markers.length);
  expect(userIndexes).toEqual([...userIndexes].sort((a, b) => a - b));
}

async function seedBusyQueueTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<{ session: SessionPage; taskId: string; sessionId: string }> {
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
  const loadedTask = await apiClient.getTask(task.id);
  if (!loadedTask.primary_session_id) throw new Error("task did not have a primary session");
  return { session, taskId: task.id, sessionId: loadedTask.primary_session_id };
}

test("mobile full queue stays usable while removing and clearing messages", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { session } = await seedFullQueueTask(
    testPage,
    apiClient,
    seedData,
    "Mobile queue management",
  );

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

test("mobile Send Now resumes Auto-run in targeted order without overflow", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);

  const { session, taskId, sessionId } = await seedBusyQueueTask(testPage, apiClient, seedData);
  const chat = session.activeChat();
  const markerA = "mobile targeted A response";
  const markerB = "mobile targeted B response";
  const markerC = "mobile targeted C response";
  for (const message of [
    scriptedQueueMessage(markerA),
    scriptedQueueMessage(markerB, 1_000),
    scriptedQueueMessage(markerC),
  ]) {
    await apiClient.queueMessage(taskId, sessionId, message);
  }

  await chat.getByTestId("queue-chip").tap();
  const panel = chat.getByTestId("queued-ghost-list");
  await expect(panel.getByTestId("queue-entry-text")).toHaveCount(3);
  const rowSendNow = panel.getByTestId("queue-entry-send-now").nth(1);
  const autoRun = panel.getByTestId("queue-auto-run");
  await expectTouchTarget(rowSendNow);
  await expectEffectiveTouchTarget(autoRun);
  await expect(autoRun).toHaveAttribute("data-state", "checked");
  await autoRun.tap();
  await expect(autoRun).toHaveAttribute("data-state", "unchecked");

  await assertNoDocumentHorizontalOverflow(testPage);

  await rowSendNow.tap();
  await expect(panel.getByTestId("queue-entry-text")).toHaveCount(2, { timeout: 10_000 });
  await expect(panel.getByTestId("queue-entry-text").nth(0)).toContainText(markerA);
  await expect(panel.getByTestId("queue-entry-text").nth(1)).toContainText(markerC);
  await expect(autoRun).toHaveAttribute("data-state", "checked", { timeout: 10_000 });

  await expectSeparateTurnsInOrder(session.chat, [markerB, markerA, markerC]);
  await session.waitForChatIdle({ timeout: 45_000 });
  await expect(panel).not.toBeVisible({ timeout: 15_000 });
  await expect(session.chat).not.toContainText("Turn cancelled by user");
  await assertNoDocumentHorizontalOverflow(testPage);
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
  await expectEffectiveTouchTarget(panel.getByTestId("queue-auto-run"));
  await expectTouchTarget(panel.getByTestId("queue-clear-all"));
  await expectTouchTarget(panel.getByTestId("queue-close"));
});
