import { test, expect } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import {
  openSeededAgentReply,
  openSeededQuickChatReply,
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

test.describe("Agent message comments on mobile", () => {
  test("opens the native drawer and Run sends the comment immediately", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const { task, body } = await openSeededAgentReply(
      testPage,
      apiClient,
      seedData,
      "Mobile Agent Message Comments",
    );

    await selectAgentReplyText(body);
    const commentTrigger = testPage.getByTestId("agent-message-comment-trigger");
    await expect(commentTrigger).toBeVisible();
    await expectTouchTarget(commentTrigger);
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
              message.content.includes("settled answer") &&
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

    await selectAgentReplyText(body);
    await testPage.getByTestId("agent-message-comment-trigger").click();
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
