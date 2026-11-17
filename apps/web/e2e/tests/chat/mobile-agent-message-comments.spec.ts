import { test, expect } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import {
  openSeededWaitingAgentReply,
  openSeededQuickChatReply,
  SELECTED_REPLY_TEXT,
  selectAgentReplyText,
} from "./agent-message-comments-helpers";

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  if (!box) throw new Error("Expected a visible touch target");
  expect(box.height).toBeGreaterThanOrEqual(44);
}

async function expectDrawerContained(page: Page, drawer: Locator) {
  const viewport = page.viewportSize();
  if (!viewport) throw new Error("Expected a mobile viewport");
  await expect
    .poll(async () => {
      const box = await drawer.boundingBox();
      return box ? box.y + box.height : Number.POSITIVE_INFINITY;
    })
    .toBeLessThanOrEqual(viewport.height + 1);

  const box = await drawer.boundingBox();
  if (!box) throw new Error("Expected a visible comment drawer");
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.y).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width);
  expect(box.y + box.height).toBeLessThanOrEqual(viewport.height + 1);
}

async function expectCommentInUtilityRow(chat: Locator, trigger: Locator) {
  const controls = chat.getByTestId("chat-status-bar-right-controls");
  // The comment action joins the transcript utility icons instead of adding a row.
  await expect(controls.getByTestId("agent-message-comment-trigger")).toHaveCount(1);
  const triggerBox = await trigger.boundingBox();
  const utilityBox = await controls
    .locator("button:visible")
    .filter({ hasNot: chat.page().getByTestId("agent-message-comment-trigger") })
    .first()
    .boundingBox();
  if (!triggerBox || !utilityBox) throw new Error("Expected measurable comment and utility icons");
  expect(
    Math.abs(triggerBox.y + triggerBox.height / 2 - (utilityBox.y + utilityBox.height / 2)),
  ).toBeLessThanOrEqual(2);
  expect(triggerBox.width).toBeLessThanOrEqual(48);
}

test.describe("Agent message comments on mobile", () => {
  test("opens the native drawer and Run sends the comment immediately", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const { task, body } = await openSeededWaitingAgentReply(
      testPage,
      apiClient,
      seedData,
      "Mobile Agent Message Comments",
    );

    // The fixture seeds a completed agent turn in a waiting session, so Run
    // sends directly and does not depend on mock-agent completion timing.
    await selectAgentReplyText(body, SELECTED_REPLY_TEXT);
    const chat = body.locator('xpath=ancestor::*[@data-testid="session-chat"][1]');
    const commentTrigger = chat.getByTestId("agent-message-comment-trigger");
    await expect(commentTrigger).toBeVisible();
    await expectTouchTarget(commentTrigger);
    await expectCommentInUtilityRow(chat, commentTrigger);
    expect(await testPage.evaluate(() => window.getSelection()?.toString())).toBe(
      SELECTED_REPLY_TEXT,
    );
    await commentTrigger.click();
    const drawer = testPage.getByTestId("agent-message-comment-drawer");
    await expect(drawer).toBeVisible();
    await expectDrawerContained(testPage, drawer);
    const runButton = drawer.getByTestId("agent-message-comment-run");
    const addButton = drawer.getByTestId("agent-message-comment-add");
    await expectTouchTarget(runButton);
    await expectTouchTarget(addButton);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await drawer.getByTestId("agent-message-comment-input").fill("Run with this correction.");
    await runButton.click();

    await expect(drawer).not.toBeVisible({ timeout: 15_000 });
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(task.session_id!);
          return messages.some(
            (message) =>
              message.author_type === "user" &&
              message.content.includes("### Agent Message Comments") &&
              message.content.includes(SELECTED_REPLY_TEXT) &&
              message.content.includes("Run with this correction."),
          );
        },
        { timeout: 20_000 },
      )
      .toBe(true);
  });

  test("keeps the comment drawer interactive inside Quick Chat", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const { dialog, body } = await openSeededQuickChatReply(
      testPage,
      apiClient,
      seedData,
      "Mobile Quick Chat Message Comments",
    );

    await selectAgentReplyText(body, SELECTED_REPLY_TEXT);
    await dialog.getByTestId("agent-message-comment-trigger").click();
    const drawer = testPage.getByTestId("agent-message-comment-drawer");
    await expect(drawer).toBeVisible();
    const input = drawer.getByTestId("agent-message-comment-input");
    await input.fill("Keep this mobile Quick Chat context.");
    await expect(input).toHaveValue("Keep this mobile Quick Chat context.");
    await drawer.getByTestId("agent-message-comment-add").click();

    await expect(drawer).not.toBeVisible();
    await expect(dialog.getByText("1 message comment", { exact: true })).toBeVisible();
  });
});
