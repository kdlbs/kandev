import { type Page } from "@playwright/test";
import path from "node:path";
import fs from "node:fs";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";
import { watchWs } from "../../helpers/causal-waits";

// Download is wired in file-context-menu.tsx → useFileOperations.downloadFile →
// downloadFileContent → triggerFileDownload (Blob + <a download>). We drive
// the visible flow: right-click a file, pick Download, assert the browser
// download event fires with the right filename and content.

async function setupTask({
  testPage,
  apiClient,
  seedData,
  profileName,
  taskTitle,
  requiredPath,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: { workspaceId: string; workflowId: string; startStepId: string; repositoryId: string };
  profileName: string;
  taskTitle: string;
  requiredPath: string;
}) {
  const profile = await createStandardProfile(apiClient, profileName);
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, taskTitle, profile.id, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });

  let workspacePath = "";
  await expect
    .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.status ?? null, {
      timeout: 30_000,
      message: `Waiting for ${taskTitle} task environment to be ready`,
    })
    .toBe("ready");
  await expect
    .poll(
      async () => {
        const environment = await apiClient.getTaskEnvironment(task.id);
        workspacePath = environment?.repos?.[0]?.worktree_path || environment?.workspace_path || "";
        return Boolean(workspacePath && fs.existsSync(path.join(workspacePath, requiredPath)));
      },
      { timeout: 60_000, message: `Waiting for ${taskTitle} worktree materialization` },
    )
    .toBe(true);
  const gateway = watchWs(testPage);
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  const treeResponse = gateway.waitForResponse("workspace.tree.get");
  await testPage.reload();
  await session.waitForLoad();
  await treeResponse;
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
    git.exec("git checkout main");
    const fileName = "download-me.txt";
    const fileContent = "kandev download test payload";
    git.createFile(fileName, fileContent);
    git.stageAll();
    git.commit("seed download file");
    git.exec("git push origin main");

    const session = await setupTask({
      testPage,
      apiClient,
      seedData,
      profileName: "ft-dl",
      taskTitle: "FT Download",
      requiredPath: fileName,
    });

    const node = await session.fileTree.waitForFileTreeNode(fileName);

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
    git.exec("git checkout main");
    // Seed a directory (via a file inside it).
    git.createFile("subdir/inside.txt", "child");
    git.stageAll();
    git.commit("seed subdir");
    git.exec("git push origin main");

    const session = await setupTask({
      testPage,
      apiClient,
      seedData,
      profileName: "ft-dl-dir",
      taskTitle: "FT Download Dir",
      requiredPath: "subdir/inside.txt",
    });

    const dirNode = await session.fileTree.waitForFileTreeNode("subdir");
    await dirNode.click({ button: "right" });

    // The menu itself must render (Delete/Rename are still available), but
    // Download must not be part of it for a directory.
    const renameItem = testPage.getByRole("menuitem", { name: "Rename" });
    await expect(renameItem).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByRole("menuitem", { name: "Download" })).toHaveCount(0);
  });
});
