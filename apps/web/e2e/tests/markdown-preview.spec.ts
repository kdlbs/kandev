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
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  await testPage.goto(`/s/${task.session_id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  return { session, sessionId: task.session_id };
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

  test("markdown preview state is persisted to sessionStorage", async ({
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
    await session.clickTab("Files");
    await expect(session.files).toBeVisible({ timeout: 5_000 });
    const fileRow = session.files.getByText("persist-test.md");
    await expect(fileRow).toBeVisible({ timeout: 10_000 });
    await fileRow.click();

    const editorTab = testPage.locator(".dv-default-tab:has-text('persist-test.md')");
    await expect(editorTab).toBeVisible({ timeout: 10_000 });

    const previewToggle = testPage.getByTestId("markdown-preview-toggle").first();
    await expect(previewToggle).toBeVisible({ timeout: 10_000 });
    await previewToggle.click();

    const preview = testPage.getByTestId("markdown-preview");
    await expect(preview).toBeVisible({ timeout: 5_000 });

    // Verify the markdownPreview flag is persisted in sessionStorage
    const storedTabs = await testPage.evaluate((sid) => {
      const raw = window.sessionStorage.getItem(`kandev.openFiles.${sid}`);
      return raw ? JSON.parse(raw) : null;
    }, sessionId);
    expect(storedTabs).not.toBeNull();
    const mdTab = storedTabs.find((t: { path: string }) => t.path.endsWith("persist-test.md"));
    expect(mdTab).toBeTruthy();
    expect(mdTab.markdownPreview).toBe(true);

    // Toggle preview off and verify the flag is cleared
    const codeToggle = testPage.getByTestId("markdown-preview-toggle").first();
    await codeToggle.click();
    await expect(preview).not.toBeVisible({ timeout: 5_000 });

    const storedTabsAfter = await testPage.evaluate((sid) => {
      const raw = window.sessionStorage.getItem(`kandev.openFiles.${sid}`);
      return raw ? JSON.parse(raw) : null;
    }, sessionId);
    const mdTabAfter = storedTabsAfter.find((t: { path: string }) =>
      t.path.endsWith("persist-test.md"),
    );
    expect(mdTabAfter.markdownPreview).toBeFalsy();
  });
});
