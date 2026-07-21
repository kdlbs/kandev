import { test, expect } from "../../fixtures/test-base";
import {
  AGENT_REPLY,
  openSeededAgentReply,
  openSeededQuickChatReply,
  selectAgentReplyText,
} from "./agent-message-comments-helpers";

test.describe("Agent message comments", () => {
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

    await selectAgentReplyText(body);
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
    const highlight = body.locator("mark.comment-highlight[data-agent-message-comment-id]");
    await expect(highlight).toHaveCount(1);
    const highlightColors = await highlight.evaluate((element) => ({
      foreground: getComputedStyle(element).color,
      prose: getComputedStyle(element.parentElement!).color,
    }));
    expect(highlightColors.foreground).toBe(highlightColors.prose);
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
    await expect(
      restoredBody.locator("mark.comment-highlight[data-agent-message-comment-id]"),
    ).toHaveCount(1);
    await expect(restoredBody.locator(".comment-badge[data-comment-id]")).toHaveCount(1);
    await expect(
      session.activeChat().getByText("1 message comment", { exact: true }),
    ).toBeVisible();

    // Saved comments use the same click-to-edit/delete loop as plan comments.
    await restoredBody.locator(".comment-badge[data-comment-id] svg").click();
    await expect(popover).toBeVisible();
    const input = popover.getByTestId("agent-message-comment-input");
    await expect(input).toHaveValue("Please expand this detail.");
    await input.fill("Please make this detail concrete.");
    await popover.getByRole("button", { name: "Update", exact: true }).click();

    await restoredBody.locator(".comment-badge[data-comment-id] svg").click();
    await expect(input).toHaveValue("Please make this detail concrete.");
    await popover.getByRole("button", { name: "Delete comment" }).click();
    await expect(restoredBody.locator("mark.comment-highlight[data-comment-id]")).toHaveCount(0);
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

    await selectAgentReplyText(body);
    await testPage.getByTestId("agent-message-comment-trigger").click();
    const popover = testPage.getByTestId("agent-message-comment-popover");
    await expect(popover).toBeVisible();
    await popover
      .getByTestId("agent-message-comment-input")
      .fill("Keep this context in Quick Chat.");
    await popover.getByTestId("agent-message-comment-add").click();
    await expect(dialog.getByText("1 message comment", { exact: true })).toBeVisible();
    await expect(body.locator("mark.comment-highlight[data-agent-message-comment-id]")).toHaveCount(
      1,
    );
    await expect(body.locator(".comment-badge[data-comment-id]")).toHaveCount(1);
  });
});
