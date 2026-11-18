import fs from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import { expect } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";

export const MARKDOWN_SELECTION_CONTENT = [
  "# Selection comments",
  "",
  "First source paragraph for a comment.",
  "",
  "Second source paragraph for a comment.",
  "",
  ...Array.from({ length: 18 }, (_, index) => [`Extra preview paragraph ${index + 1}.`, ""]).flat(),
  "",
].join("\n");
export const MARKDOWN_LAST_SELECTION_SENTINEL = "Extra preview paragraph 18.";

export async function openMarkdownPreviewForSelectionComment({
  page,
  apiClient,
  seedData,
  backend,
  fileName,
  title,
}: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  fileName: string;
  title: string;
}) {
  const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
  const diskPath = path.join(repoDir, fileName);
  fs.writeFileSync(diskPath, MARKDOWN_SELECTION_CONTENT);

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  const isMobile = (page.viewportSize()?.width ?? Number.POSITIVE_INFINITY) < 768;
  const filesTab = page.getByRole("button", { name: "Files", exact: true });
  if (await filesTab.isVisible()) {
    if (isMobile) await filesTab.tap();
    else await filesTab.click();
  } else await session.clickTab("Files");
  await expect(session.files).toBeVisible({ timeout: 10_000 });
  const file = await session.fileTree.waitForFileTreeNode(fileName, 30_000);
  if (isMobile) await file.tap();
  else await file.click();

  if (isMobile) {
    await expect(page.getByTestId("mobile-file-viewer-panel")).toBeVisible({ timeout: 15_000 });
  } else {
    const editorTab = page.locator(`.dv-default-tab:has-text('${fileName}')`);
    await expect(editorTab).toBeVisible({ timeout: 15_000 });
  }
  const previewToggle = page.getByTestId("markdown-preview-toggle").first();
  await expect(previewToggle).toBeVisible({ timeout: 10_000 });
  await previewToggle.click();

  const preview = page.getByTestId("markdown-preview");
  await expect(preview).toBeVisible({ timeout: 10_000 });
  return { task, session, sessionId: task.session_id, preview, diskPath };
}

export async function selectMarkdownPreviewTextWithTouch(
  target: import("@playwright/test").Locator,
  quote: string,
) {
  await target.evaluate((element, selectedText) => {
    element.dispatchEvent(new Event("touchstart", { bubbles: true, cancelable: true }));
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    let node = walker.nextNode();
    while (node) {
      const text = node.nodeValue ?? "";
      const start = text.indexOf(selectedText);
      if (start >= 0) {
        const range = document.createRange();
        range.setStart(node, start);
        range.setEnd(node, start + selectedText.length);
        const selection = window.getSelection();
        if (!selection) throw new Error("Browser selection is unavailable");
        selection.removeAllRanges();
        selection.addRange(range);
        document.dispatchEvent(new Event("selectionchange", { bubbles: true }));
        element.dispatchEvent(new Event("touchend", { bubbles: true, cancelable: true }));
        return;
      }
      node = walker.nextNode();
    }
    throw new Error(`Could not find selected text: ${selectedText}`);
  }, quote);
}

export async function readStoredFileComments(page: Page, sessionId: string) {
  return page.evaluate((id) => {
    const raw = window.sessionStorage.getItem(`kandev.comments.${id}`);
    return raw ? (JSON.parse(raw) as Array<Record<string, unknown>>) : [];
  }, sessionId);
}

export async function expectStoredMarkdownComment({
  page,
  sessionId,
  fileName,
  quote,
  commentText,
  startLine,
  endLine,
  repositoryId,
}: {
  page: Page;
  sessionId: string;
  fileName: string;
  quote: string;
  commentText: string;
  startLine: number;
  endLine: number;
  repositoryId: string;
}) {
  await expect
    .poll(async () => {
      const comments = await readStoredFileComments(page, sessionId);
      return comments.some(
        (comment) =>
          comment.source === "diff" &&
          comment.status === "pending" &&
          comment.filePath === fileName &&
          comment.repositoryId === repositoryId &&
          comment.codeContent === quote &&
          comment.text === commentText &&
          comment.startLine === startLine &&
          comment.endLine === endLine,
      );
    })
    .toBe(true);
}
