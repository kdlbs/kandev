import fs from "node:fs";
import path from "node:path";
import { type Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";

/**
 * Seed a task with the simple-message scenario and navigate to its session page.
 */
async function seedSimpleTask(
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

  return { session, sessionId: task.session_id ?? task.id };
}

/**
 * Seed a task using the symlink-file-setup mock scenario. The scenario creates
 * real-file.txt, a symlink link-file.txt → real-file.txt, commits both, then
 * modifies real-file.txt leaving an uncommitted diff.
 */
async function seedSymlinkDiffTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Symlink Diff E2E",
    seedData.agentProfileId,
    {
      description: "/e2e:symlink-file-setup",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();

  await expect(session.chat.getByText("symlink-file-setup complete", { exact: false })).toBeVisible(
    { timeout: 45_000 },
  );

  return session;
}

test.describe("Symlink file handling", () => {
  test.describe.configure({ retries: 1 });

  test("symlink to a directory appears as expandable folder in the file tree", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Create a directory with a child file, and a symlink pointing to it.
    // BEFORE navigating so the file tree picks it up on initial load.
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");

    fs.mkdirSync(path.join(repoDir, "real-dir"), { recursive: true });
    fs.writeFileSync(path.join(repoDir, "real-dir", "child.txt"), "inside symlinked dir\n");
    fs.symlinkSync("real-dir", path.join(repoDir, "link-dir"));

    const { session } = await seedSimpleTask(
      testPage,
      apiClient,
      seedData,
      "Symlink Dir Tree Test",
    );

    // Open Files tab
    await session.clickTab("Files");
    await expect(session.files).toBeVisible({ timeout: 5_000 });

    // The symlink-to-directory should appear in the tree
    const linkDirRow = session.files.getByText("link-dir");
    await expect(linkDirRow).toBeVisible({ timeout: 10_000 });

    // Click to expand — if the fix is missing, this is classified as a file
    // and would try to open it in the editor instead of expanding.
    await linkDirRow.click();

    // Assert the child file inside the symlinked directory is now visible
    const childFile = session.files.getByText("child.txt");
    await expect(childFile).toBeVisible({ timeout: 10_000 });
  });

  test("clicking a symlink to a file opens the file editor", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");

    fs.writeFileSync(path.join(repoDir, "real-file.txt"), "Hello from symlink target!\n");
    fs.symlinkSync("real-file.txt", path.join(repoDir, "link-file.txt"));

    const { session } = await seedSimpleTask(
      testPage,
      apiClient,
      seedData,
      "Symlink File Tree Test",
    );

    // Open Files tab and click the symlink
    await session.clickTab("Files");
    await expect(session.files).toBeVisible({ timeout: 5_000 });

    const fileRow = session.files.getByText("link-file.txt");
    await expect(fileRow).toBeVisible({ timeout: 10_000 });
    await fileRow.click();

    // Assert editor tab opens for the symlink
    const editorTab = testPage.locator(".dv-default-tab:has-text('link-file.txt')");
    await expect(editorTab).toBeVisible({ timeout: 10_000 });

    // Assert the file content is visible in the Monaco editor
    const editorContent = testPage.locator(".view-lines");
    await expect(editorContent).toContainText("Hello from symlink target", {
      timeout: 10_000,
    });
  });

  test("clicking a symlink target diff in the Changes panel opens the diff viewer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedSymlinkDiffTask(testPage, apiClient, seedData);

    // Open Changes tab
    const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
    await expect(changesTab).toBeVisible({ timeout: 10_000 });
    await changesTab.click();

    // Click real-file.txt in the changes list (has uncommitted diff)
    const fileRow = testPage
      .locator("button, [role='button'], [class*='file']")
      .filter({ hasText: "real-file.txt" })
      .first();
    await expect(fileRow).toBeVisible({ timeout: 10_000 });
    await fileRow.click();

    // Assert Pierre Diffs viewer appears with the modification
    await expect(testPage.locator("diffs-container")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("Modified symlink target", { exact: false })).toBeVisible({
      timeout: 60_000,
    });
  });
});
