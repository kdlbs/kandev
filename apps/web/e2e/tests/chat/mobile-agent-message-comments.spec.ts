import { test, expect } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import {
  openSeededWaitingAgentReply,
  openSeededQuickChatReply,
  SELECTED_REPLY_TEXT,
  selectAgentReplyTextWithTouch,
} from "./agent-message-comments-helpers";

async function expectTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  if (!box) throw new Error("Expected a visible touch target");
  expect(box.height).toBeGreaterThanOrEqual(44);
}

async function expectDockWithinOwner(locator: Locator, owner: Locator, lowerBoundary?: Locator) {
  const [dockBox, ownerBox] = await Promise.all([locator.boundingBox(), owner.boundingBox()]);
  expect(dockBox).not.toBeNull();
  expect(ownerBox).not.toBeNull();
  if (!dockBox || !ownerBox) throw new Error("Expected owner and dock geometry");
  expect(dockBox.x).toBeGreaterThanOrEqual(ownerBox.x - 1);
  expect(dockBox.x + dockBox.width).toBeLessThanOrEqual(ownerBox.x + ownerBox.width + 1);
  expect(dockBox.y).toBeGreaterThanOrEqual(ownerBox.y - 1);
  expect(dockBox.y + dockBox.height).toBeLessThanOrEqual(ownerBox.y + ownerBox.height + 1);
  if (lowerBoundary) {
    const boundaryBox = await lowerBoundary.boundingBox();
    expect(boundaryBox).not.toBeNull();
    if (boundaryBox) expect(dockBox.y + dockBox.height).toBeLessThanOrEqual(boundaryBox.y + 1);
  }
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
    const { task, body } = await openSeededWaitingAgentReply(
      testPage,
      apiClient,
      seedData,
      "Mobile Agent Message Comments",
    );

    // The fixture seeds a completed agent turn in a waiting session, so Run
    // sends directly and does not depend on mock-agent completion timing.
    await selectAgentReplyTextWithTouch(body, SELECTED_REPLY_TEXT);
    const commentTrigger = testPage.getByTestId("agent-message-comment-trigger");
    await expect(commentTrigger).toBeVisible();
    await expectTouchTarget(commentTrigger);
    const dock = testPage.getByTestId("selection-comment-dock");
    const owner = body.locator("xpath=ancestor::*[contains(@class,'chat-message-list')][1]");
    const composer = testPage.locator(".tiptap.ProseMirror").last();
    await expectDockWithinOwner(dock, owner, composer);
    await commentTrigger.tap();
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

    await selectAgentReplyTextWithTouch(body, SELECTED_REPLY_TEXT);
    const commentTrigger = testPage.getByTestId("agent-message-comment-trigger");
    const dock = testPage.getByTestId("selection-comment-dock");
    const owner = body.locator("xpath=ancestor::*[contains(@class,'chat-message-list')][1]");
    const composer = dialog.locator(".tiptap.ProseMirror").last();
    await expectDockWithinOwner(dock, owner, composer);
    await commentTrigger.tap();
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
