import fs from "node:fs";
import { test, expect } from "../../fixtures/test-base";
import {
  expectStoredMarkdownComment,
  MARKDOWN_SELECTION_CONTENT,
  MARKDOWN_LAST_SELECTION_SENTINEL,
  openMarkdownPreviewForSelectionComment,
  selectMarkdownPreviewTextWithTouch,
} from "../../helpers/markdown-preview-comments";
import {
  restoreSelectionCommentVisualViewport,
  setSelectionCommentVisualViewport,
} from "../../helpers/selection-comment-visual-viewport";

const SELECTED_TEXT = "Second source paragraph for a comment.";
const COMMENT_TEXT = "Clarify this source paragraph.";

test.describe("Markdown selection comments on mobile", () => {
  test("docks touch selection and creates a pending comment with its source lines", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(90_000);
    const fileName = `mobile-selection-comment-${Date.now()}.md`;
    const { session, sessionId, preview, diskPath } = await openMarkdownPreviewForSelectionComment({
      page: testPage,
      apiClient,
      seedData,
      backend,
      fileName,
      title: "Mobile Markdown Selection Comment",
    });

    const paragraph = preview.getByTestId("markdown-preview-source-block").filter({
      hasText: SELECTED_TEXT,
    });
    await expect(paragraph).toBeVisible();
    await selectMarkdownPreviewTextWithTouch(paragraph, SELECTED_TEXT);

    const dock = testPage.getByTestId("selection-comment-dock");
    const action = testPage.getByTestId("markdown-preview-comment-button");
    await expect(action).toBeVisible({ timeout: 5_000 });
    const [dockBox, ownerBox, actionBox] = await Promise.all([
      dock.boundingBox(),
      preview.locator(".flex-1.overflow-auto").boundingBox(),
      action.boundingBox(),
    ]);
    expect(dockBox).not.toBeNull();
    expect(ownerBox).not.toBeNull();
    expect(actionBox).not.toBeNull();
    if (!dockBox || !ownerBox || !actionBox) throw new Error("Expected touch dock geometry");
    expect(actionBox.height).toBeGreaterThanOrEqual(44);
    expect(dockBox.x).toBeGreaterThanOrEqual(ownerBox.x - 1);
    expect(dockBox.x + dockBox.width).toBeLessThanOrEqual(ownerBox.x + ownerBox.width + 1);
    expect(dockBox.y + dockBox.height).toBeLessThanOrEqual(ownerBox.y + ownerBox.height + 1);
    const reservedPadding = await preview
      .locator(".flex-1.overflow-auto")
      .evaluate((owner) => Number.parseFloat(window.getComputedStyle(owner).paddingBottom));
    expect(reservedPadding).toBeGreaterThanOrEqual(56);

    const owner = preview.locator(".flex-1.overflow-auto");
    const beforeViewportChange = await owner.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      return { innerHeight: window.innerHeight, top: rect.top, bottom: rect.bottom };
    });
    await setSelectionCommentVisualViewport(testPage, {
      height: Math.max(320, Math.floor(beforeViewportChange.innerHeight * 0.62)),
      offsetTop: 24,
    });
    const afterViewportChange = await owner.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      return { innerHeight: window.innerHeight, top: rect.top, bottom: rect.bottom };
    });
    expect(afterViewportChange.innerHeight).toBe(beforeViewportChange.innerHeight);
    expect(afterViewportChange.top).toBeCloseTo(beforeViewportChange.top, 1);
    expect(afterViewportChange.bottom).toBeCloseTo(beforeViewportChange.bottom, 1);
    const finalParagraph = preview.getByText(MARKDOWN_LAST_SELECTION_SENTINEL, { exact: true });
    await owner.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await finalParagraph.scrollIntoViewIfNeeded();
    const [clippedDockBox, finalParagraphBox] = await Promise.all([
      dock.boundingBox(),
      finalParagraph.boundingBox(),
    ]);
    expect(clippedDockBox).not.toBeNull();
    expect(finalParagraphBox).not.toBeNull();
    if (!clippedDockBox || !finalParagraphBox) throw new Error("Expected clipped preview geometry");
    expect(finalParagraphBox.y + finalParagraphBox.height).toBeLessThanOrEqual(
      clippedDockBox.y + 1,
    );

    await testPage.screenshot({ path: testInfo.outputPath("markdown-selection-comment-dock.png") });
    await restoreSelectionCommentVisualViewport(testPage);
    await action.tap();

    const popover = testPage.locator("[data-markdown-comment-popover] .fixed.z-50");
    await expect(popover).toBeVisible();
    await expect(popover.locator("pre")).toContainText(SELECTED_TEXT);
    const popoverBox = await popover.boundingBox();
    const viewport = testPage.viewportSize();
    expect(popoverBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    if (!popoverBox || !viewport) throw new Error("Expected mobile comment editor geometry");
    expect(popoverBox.x).toBeGreaterThanOrEqual(0);
    expect(popoverBox.y).toBeGreaterThanOrEqual(0);
    expect(popoverBox.x + popoverBox.width).toBeLessThanOrEqual(viewport.width + 1);
    expect(popoverBox.y + popoverBox.height).toBeLessThanOrEqual(viewport.height + 1);

    await popover.getByPlaceholder("Add your comment or instruction...").fill(COMMENT_TEXT);
    await popover.getByRole("button", { name: "Add", exact: true }).tap();
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

    await testPage.getByRole("button", { name: "Chat", exact: true }).tap();
    await expect(session.activeChat().getByText(`${fileName} (1)`, { exact: true })).toBeVisible();
    expect(fs.readFileSync(diskPath, "utf8")).toBe(MARKDOWN_SELECTION_CONTENT);
    await restoreSelectionCommentVisualViewport(testPage);
  });
});
