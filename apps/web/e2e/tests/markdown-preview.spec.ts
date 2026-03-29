import fs from "node:fs";
import path from "node:path";
import { type Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";

const MARKDOWN_CONTENT = `# Hello World

This is a **markdown** file with some content.

- Item 1
- Item 2
- Item 3

\`\`\`js
console.log("hello");
\`\`\`
`;

async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ session: SessionPage; sessionId: string }> {
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
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  return { session, sessionId: task.session_id };
}

/** Open a markdown file from the Files panel and enable preview mode. */
async function openFileInPreview(
  testPage: Page,
  session: SessionPage,
  fileName: string,
): Promise<void> {
  await session.clickTab("Files");
  await expect(session.files).toBeVisible({ timeout: 5_000 });
  const fileRow = session.files.getByText(fileName);
  await expect(fileRow).toBeVisible({ timeout: 10_000 });
  await fileRow.click();

  const editorTab = testPage.locator(`.dv-default-tab:has-text('${fileName}')`);
  await expect(editorTab).toBeVisible({ timeout: 10_000 });

  const previewToggle = testPage.getByTestId("markdown-preview-toggle").first();
  await expect(previewToggle).toBeVisible({ timeout: 10_000 });
  await previewToggle.click();

  await expect(testPage.getByTestId("markdown-preview")).toBeVisible({ timeout: 5_000 });
}

test.describe("Markdown preview", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("toggle markdown preview in file editor", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    // Create a markdown file in the workspace repo before navigating
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const filePath = path.join(repoDir, "readme.md");
    fs.writeFileSync(filePath, MARKDOWN_CONTENT);

    const { session } = await seedTaskWithSession(
      testPage,
      apiClient,
      seedData,
      "Markdown Preview Test",
    );

    // Open the Files panel and click on the markdown file
    await session.clickTab("Files");
    await expect(session.files).toBeVisible({ timeout: 5_000 });
    const fileRow = session.files.getByText("readme.md");
    await expect(fileRow).toBeVisible({ timeout: 10_000 });
    await fileRow.click();

    // Wait for the file editor tab to appear
    const editorTab = testPage.locator(".dv-default-tab:has-text('readme.md')");
    await expect(editorTab).toBeVisible({ timeout: 10_000 });

    // The preview toggle button should be visible (only for markdown files)
    const previewToggle = testPage.getByTestId("markdown-preview-toggle").first();
    await expect(previewToggle).toBeVisible({ timeout: 10_000 });

    // Click to enable markdown preview
    await previewToggle.click();

    // The markdown preview should be visible with rendered content
    const preview = testPage.getByTestId("markdown-preview");
    await expect(preview).toBeVisible({ timeout: 5_000 });
    // Check that markdown is rendered (heading should be an <h1>)
    await expect(preview.locator("h1")).toContainText("Hello World");
    // Check that list items are rendered
    await expect(preview.locator("li")).toHaveCount(3);

    // Capture screenshot for PR description (only when CAPTURE_PR_ASSETS=true)
    await prCapture.screenshot("markdown-preview-on", {
      caption: "Markdown file rendered in preview mode",
    });

    // Toggle back to code view
    const codeToggle = testPage.getByTestId("markdown-preview-toggle").first();
    await codeToggle.click();

    // Preview should be gone
    await expect(preview).not.toBeVisible({ timeout: 5_000 });
  });

  test("markdown preview persists across page refresh", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Create a markdown file
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const filePath = path.join(repoDir, "persist-test.md");
    fs.writeFileSync(filePath, "# Persist Test\n\nSome content here.");

    const { session, sessionId } = await seedTaskWithSession(
      testPage,
      apiClient,
      seedData,
      "Markdown Persist Test",
    );

    // Open file and enable preview
    await openFileInPreview(testPage, session, "persist-test.md");
    await expect(testPage.getByTestId("markdown-preview").locator("h1")).toContainText(
      "Persist Test",
    );

    // Verify the markdownPreview flag is in sessionStorage
    const storedTabs = await testPage.evaluate((sid) => {
      const raw = window.sessionStorage.getItem(`kandev.openFiles.${sid}`);
      return raw ? JSON.parse(raw) : null;
    }, sessionId);
    expect(storedTabs).not.toBeNull();
    const mdTab = storedTabs.find((t: { path: string }) => t.path.endsWith("persist-test.md"));
    expect(mdTab).toBeTruthy();
    expect(mdTab.markdownPreview).toBe(true);

    // Brief pause to let the sessionStorage write settle before reload
    await testPage.waitForTimeout(500);

    // Reload the page — sessionStorage survives same-URL reload.
    // After reload, the restored file tab becomes active (not the chat),
    // so we wait for the sidebar instead of the chat panel.
    await testPage.reload();
    const sessionAfter = new SessionPage(testPage);
    await expect(sessionAfter.sidebar).toBeVisible({ timeout: 30_000 });

    // The file tab should be restored with preview still active
    const editorTabAfter = testPage.locator(".dv-default-tab:has-text('persist-test.md')");
    await expect(editorTabAfter).toBeVisible({ timeout: 15_000 });
    await editorTabAfter.click();

    const previewAfter = testPage.getByTestId("markdown-preview");
    await expect(previewAfter).toBeVisible({ timeout: 10_000 });
    await expect(previewAfter.locator("h1")).toContainText("Persist Test");
  });

  test("open markdown preview from diff toolbar", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Create a markdown file as an untracked file — it will show in Changes
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const filePath = path.join(repoDir, "preview-from-diff.md");
    fs.writeFileSync(filePath, "# Preview From Diff\n\nThis file was created for the diff test.");

    const { session } = await seedTaskWithSession(
      testPage,
      apiClient,
      seedData,
      "Markdown Diff Preview Test",
    );

    // Open Changes panel — the untracked .md file should appear in the file list.
    // Click "Changes" tab first, then click the file to open its diff.
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 5_000 });

    // Wait for git status to detect the file and show it in the changes list
    const fileEntry = session.changes.getByText("preview-from-diff.md");
    await expect(fileEntry).toBeVisible({ timeout: 15_000 });
    await fileEntry.click();

    // The diff opens as a center panel tab "Diff [preview-from-diff.md]".
    // Wait for the diff tab and its content to render.
    const diffTab = testPage.locator(".dv-default-tab:has-text('preview-from-diff.md')");
    await expect(diffTab).toBeVisible({ timeout: 10_000 });

    // The diff content should show our markdown text
    await expect(testPage.getByText("# Preview From Diff")).toBeVisible({ timeout: 15_000 });

    // Click the "Preview markdown" button (eye icon) in the diff header toolbar.
    // The button is in the center diff panel, not the right-side changes panel.
    const previewBtn = testPage.getByRole("button", { name: "Preview markdown" }).first();
    await expect(previewBtn).toBeVisible({ timeout: 10_000 });
    await previewBtn.click();

    // The file editor should open in preview mode (not code mode).
    // The markdown preview should be active immediately (no need to toggle).
    const preview = testPage.getByTestId("markdown-preview");
    await expect(preview).toBeVisible({ timeout: 10_000 });
    await expect(preview.locator("h1")).toContainText("Preview From Diff");
  });
});
