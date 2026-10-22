import { test, expect } from "../../fixtures/test-base";
import {
  AGENT_REPLY,
  SELECTED_REPLY_TEXT,
  clickAgentMessageHighlight,
  expectAgentMessageHighlight,
  openSeededAgentReply,
  openSeededWaitingAgentReply,
  openSeededQuickChatReply,
  selectAgentReplyText,
} from "./agent-message-comments-helpers";

test.describe("Agent message comments", () => {
  for (const surface of ["Task Chat", "Quick Chat"] as const) {
    // @covers AC-UI-AGENT-MESSAGE-COMMENTS-001.3
    test(`shows the comment action after a drag ends in ${surface} message padding`, async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      const openReply =
        surface === "Task Chat" ? openSeededWaitingAgentReply : openSeededQuickChatReply;
      const { body } = await openReply(
        testPage,
        apiClient,
        seedData,
        `${surface} selection padding`,
      );
      await body.scrollIntoViewIfNeeded();
      const selectedText = "The settled answer";
      const points = await body.evaluate((element, quote) => {
        const node = element.querySelector("p")!.firstChild!;
        const range = document.createRange();
        range.setStart(node, quote.length);
        range.collapse(true);
        const caret = range.getBoundingClientRect();
        return {
          startX: caret.x,
          endX: element.getBoundingClientRect().left - 4,
          y: caret.y + caret.height / 2,
        };
      }, selectedText);
      await testPage.mouse.move(points.startX, points.y);
      await testPage.mouse.down();
      await testPage.mouse.move(points.endX, points.y, { steps: 12 });
      await testPage.mouse.up();

      expect(await testPage.evaluate(() => window.getSelection()?.toString())).toBe(selectedText);
      const trigger = testPage.getByTestId("agent-message-comment-trigger");
      await expect(trigger).toBeVisible();
      await trigger.click();
      const popover = testPage.getByTestId("agent-message-comment-popover");
      await expect(popover).toBeVisible();
      await expect(popover).toContainText(selectedText);
    });
  }

  test("keeps the comment action dismissed after an unrelated selection-preserving click", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { body } = await openSeededWaitingAgentReply(
      testPage,
      apiClient,
      seedData,
      "Selection dismissal",
    );
    // Model a toolbar control that preserves the native selection on mousedown.
    await body.evaluate((element) => {
      const button = document.createElement("button");
      button.textContent = "Unrelated selection-preserving control";
      button.addEventListener("mousedown", (event) => event.preventDefault());
      element.parentElement!.append(button);
    });
    await selectAgentReplyText(body, SELECTED_REPLY_TEXT);
    const trigger = testPage.getByTestId("agent-message-comment-trigger");
    await expect(trigger).toBeVisible();
    await testPage.getByRole("button", { name: "Unrelated selection-preserving control" }).click();

    expect(await testPage.evaluate(() => window.getSelection()?.toString())).toBe(
      SELECTED_REPLY_TEXT,
    );
    await expect(trigger).not.toBeVisible();
  });

  test("adds pending context and restores the highlight after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const { session, body } = await openSeededAgentReply(
      testPage,
      apiClient,
      seedData,
      "Agent Message Comments",
    );

    await selectAgentReplyText(body, SELECTED_REPLY_TEXT);
    const commentTrigger = testPage.getByTestId("agent-message-comment-trigger");
    await expect(commentTrigger).toBeVisible();
    const popover = testPage.getByTestId("agent-message-comment-popover");
    await expect(popover).not.toBeVisible();
    await commentTrigger.click();
    await expect(popover).toBeVisible();
    await popover.getByTestId("agent-message-comment-input").fill("Please expand this detail.");
    await popover.getByTestId("agent-message-comment-add").click();

    await expect(popover).not.toBeVisible();
    await expect(
      session.activeChat().getByText("1 message comment", { exact: true }),
    ).toBeVisible();
    await expectAgentMessageHighlight(body, 1);
    await expect(body.locator("mark[data-agent-message-comment-id]")).toHaveCount(0);
    await expect(body.locator(".comment-badge[data-comment-id]")).toHaveCount(1);

    await testPage.reload();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    const restoredBody = session
      .activeChat()
      .locator(`[data-agent-message-body][data-message-id]`)
      .filter({
        hasText: AGENT_REPLY,
      });
    await expectAgentMessageHighlight(restoredBody, 1);
    await expect(restoredBody.locator(".comment-badge[data-comment-id]")).toHaveCount(1);
    await expect(
      session.activeChat().getByText("1 message comment", { exact: true }),
    ).toBeVisible();

    // Saved highlights and badges both use the same edit/delete loop as plan comments.
    await clickAgentMessageHighlight(restoredBody, SELECTED_REPLY_TEXT);
    await expect(popover).toBeVisible();
    const input = popover.getByTestId("agent-message-comment-input");
    await expect(input).toHaveValue("Please expand this detail.");
    await input.fill("Please make this detail concrete.");
    await popover.getByRole("button", { name: "Update", exact: true }).click();

    await restoredBody.locator(".comment-badge[data-comment-id] svg").click();
    await expect(input).toHaveValue("Please make this detail concrete.");
    await popover.getByRole("button", { name: "Delete comment" }).click();
    await expectAgentMessageHighlight(restoredBody, 0);
    await expect(restoredBody.locator(".comment-badge[data-comment-id]")).toHaveCount(0);
    await expect(
      session.activeChat().getByText("1 message comment", { exact: true }),
    ).not.toBeVisible();
  });

  test("wires pending context into Quick Chat", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(90_000);
    const { dialog, body } = await openSeededQuickChatReply(
      testPage,
      apiClient,
      seedData,
      "Quick Chat Message Comments",
    );

    await selectAgentReplyText(body, SELECTED_REPLY_TEXT);
    await testPage.getByTestId("agent-message-comment-trigger").click();
    const popover = testPage.getByTestId("agent-message-comment-popover");
    await expect(popover).toBeVisible();
    await popover
      .getByTestId("agent-message-comment-input")
      .fill("Keep this context in Quick Chat.");
    await popover.getByTestId("agent-message-comment-add").click();
    await expect(dialog.getByText("1 message comment", { exact: true })).toBeVisible();
    await expectAgentMessageHighlight(body, 1);
    await expect(body.locator(".comment-badge[data-comment-id]")).toHaveCount(1);
  });
});
