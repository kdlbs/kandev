import { type Page } from "@playwright/test";
import path from "node:path";
import fs from "node:fs";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../../helpers/git-helper";

// Download is wired in file-context-menu.tsx → useFileOperations.downloadFile →
// downloadFileContent → triggerFileDownload (Blob + <a download>). We drive
// the visible flow: right-click a file, pick Download, assert the browser
// download event fires with the right filename and content.

async function setupTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: { workspaceId: string; workflowId: string; startStepId: string; repositoryId: string },
  profileName: string,
  taskTitle: string,
) {
  const profile = await createStandardProfile(apiClient, profileName);
  await apiClient.createTaskWithAgent(seedData.workspaceId, taskTitle, profile.id, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const session = await openTaskSession(testPage, taskTitle);
  await session.clickTab("Files");
  return session;
}

test.describe("File tree Download", () => {
  test("Download menu item downloads the file with its original name and content", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    const fileName = "download-me.txt";
    const fileContent = "kandev download test payload";
    git.createFile(fileName, fileContent);
    git.stageAll();
    git.commit("seed download file");

    const session = await setupTask(testPage, apiClient, seedData, "ft-dl", "FT Download");

    const node = session.fileTreeNode(fileName);
    await expect(node).toBeVisible({ timeout: 15_000 });

    await node.click({ button: "right" });
    const downloadItem = testPage.getByRole("menuitem", { name: "Download" });
    await expect(downloadItem).toBeVisible({ timeout: 5_000 });

    const downloadPromise = testPage.waitForEvent("download");
    await downloadItem.click();
    const download = await downloadPromise;

    expect(download.suggestedFilename()).toBe(fileName);

    // Read the downloaded bytes and verify they match the seeded content.
    const tmpPath = path.join(backend.tmpDir, `dl-${Date.now()}.txt`);
    await download.saveAs(tmpPath);
    expect(fs.readFileSync(tmpPath, "utf8")).toBe(fileContent);
  });

  test("Download is hidden for directory nodes", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    // Seed a directory (via a file inside it).
    git.createFile("subdir/inside.txt", "child");
    git.stageAll();
    git.commit("seed subdir");

    const session = await setupTask(testPage, apiClient, seedData, "ft-dl-dir", "FT Download Dir");

    const dirNode = session.fileTreeNode("subdir");
    await expect(dirNode).toBeVisible({ timeout: 15_000 });
    await dirNode.click({ button: "right" });

    // The menu itself must render (Delete/Rename are still available), but
    // Download must not be part of it for a directory.
    const renameItem = testPage.getByRole("menuitem", { name: "Rename" });
    await expect(renameItem).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByRole("menuitem", { name: "Download" })).toHaveCount(0);
  });
});
