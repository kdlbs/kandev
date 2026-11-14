import fs from "node:fs";
import { test, expect } from "../../fixtures/test-base";
import { selectMarkdownPreviewText } from "../../helpers/markdown-preview";
import {
  expectStoredMarkdownComment,
  MARKDOWN_SELECTION_CONTENT,
  openMarkdownPreviewForSelectionComment,
} from "../../helpers/markdown-preview-comments";

const SELECTED_TEXT = "Second source paragraph for a comment.";
const COMMENT_TEXT = "Clarify this source paragraph.";

test.describe("Markdown selection comments", () => {
  test("keeps desktop selection placement and creates the expected pending comment", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);
    const fileName = `selection-comment-${Date.now()}.md`;
    const { session, sessionId, preview, diskPath } = await openMarkdownPreviewForSelectionComment({
      page: testPage,
      apiClient,
      seedData,
      backend,
      fileName,
      title: "Markdown Selection Comment",
    });

    const paragraph = preview.getByTestId("markdown-preview-source-block").filter({
      hasText: SELECTED_TEXT,
    });
    await expect(paragraph).toBeVisible();
    await selectMarkdownPreviewText(paragraph);

    const action = testPage.getByTestId("markdown-preview-comment-button");
    await expect(action).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByTestId("selection-comment-dock")).toHaveCount(0);
    await action.click();

    const popover = testPage.locator("[data-markdown-comment-popover] .fixed.z-50");
    await expect(popover).toBeVisible();
    await expect(popover.locator("pre")).toContainText(SELECTED_TEXT);
    await popover.getByPlaceholder("Add your comment or instruction...").fill(COMMENT_TEXT);
    await popover.getByRole("button", { name: "Add", exact: true }).click();
    await expect(preview.getByTestId("markdown-preview-commented-range")).toBeVisible();
    await expectStoredMarkdownComment({
      page: testPage,
      sessionId,
      fileName,
      quote: SELECTED_TEXT,
      commentText: COMMENT_TEXT,
      startLine: 5,
      endLine: 5,
      repositoryId: seedData.repositoryId,
    });

    await session.clickSessionChatTab();
    await expect(session.activeChat().getByText(`${fileName} (1)`, { exact: true })).toBeVisible();
    expect(fs.readFileSync(diskPath, "utf8")).toBe(MARKDOWN_SELECTION_CONTENT);
  });
});
