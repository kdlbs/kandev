import { type Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../../helpers/git-helper";

// File creation lives in file-browser-toolbar.tsx ("New file" button) +
// inline-file-input.tsx (InlineFileInput) + file-browser.tsx
// (handleStartCreate / handleCreateFileSubmit).
// Behaviour to lock in:
//   - "New file" at the tree root creates a file at root
//   - After expanding a folder, "New file" creates inside that folder
//   - Typing a path-like name (subdir/file.ts) creates implicit parent folders
//   - Escape cancels the input
// There is no separate "create folder" affordance today; users create folders
// implicitly by including a "/" in the new-file name.

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

async function startCreateAtRoot(testPage: Page) {
  const btn = testPage.getByRole("button", { name: "New file" });
  await expect(btn).toBeVisible({ timeout: 15_000 });
  await btn.click();
  const input = testPage.getByPlaceholder("filename...");
  await expect(input).toBeVisible({ timeout: 5_000 });
  await expect(input).toBeFocused({ timeout: 2_000 });
  return input;
}

test.describe("File tree create file", () => {
  test("New file at root creates a file on disk and in the tree", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    // Seed at least one file so the tree loads. Without any files the tree
    // shows "No files found" instead of the toolbar.
    git.createFile("seed.ts", "seed");
    git.stageAll();
    git.commit("seed");

    const session = await setupTask(
      testPage,
      apiClient,
      seedData,
      "ft-create-root",
      "FT Create Root",
    );

    await expect(session.fileTreeNode("seed.ts")).toBeVisible({ timeout: 15_000 });

    const input = await startCreateAtRoot(testPage);
    await input.fill("brand-new.ts");
    await input.press("Enter");

    await expect(session.fileTreeNode("brand-new.ts")).toBeVisible({ timeout: 10_000 });
    await expect
      .poll(() => fs.existsSync(path.join(repoDir, "brand-new.ts")), { timeout: 10_000 })
      .toBe(true);
  });

  test("New file inside expanded folder creates the file in that folder", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("scope/existing.ts", "x");
    git.stageAll();
    git.commit("seed scope");

    const session = await setupTask(
      testPage,
      apiClient,
      seedData,
      "ft-create-folder",
      "FT Create In Folder",
    );

    // Expand the folder so it becomes the "active folder" for handleStartCreate.
    const folder = session.fileTreeNode("scope");
    await expect(folder).toBeVisible({ timeout: 15_000 });
    await folder.click();
    await expect(session.fileTreeNode("scope/existing.ts")).toBeVisible({ timeout: 10_000 });

    await testPage.getByRole("button", { name: "New file" }).click();
    const input = testPage.getByPlaceholder("filename...");
    await expect(input).toBeVisible({ timeout: 5_000 });
    await input.fill("inside.ts");
    await input.press("Enter");

    await expect(session.fileTreeNode("scope/inside.ts")).toBeVisible({ timeout: 10_000 });
    await expect
      .poll(() => fs.existsSync(path.join(repoDir, "scope", "inside.ts")), { timeout: 10_000 })
      .toBe(true);
  });

  test("typing path-like name creates implicit parent folder", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("seed.ts", "seed");
    git.stageAll();
    git.commit("seed");

    const session = await setupTask(
      testPage,
      apiClient,
      seedData,
      "ft-create-implicit",
      "FT Create Implicit Folder",
    );

    await expect(session.fileTreeNode("seed.ts")).toBeVisible({ timeout: 15_000 });

    const input = await startCreateAtRoot(testPage);
    await input.fill("newdir/leaf.ts");
    await input.press("Enter");

    // The file appears under the new folder on disk.
    await expect
      .poll(() => fs.existsSync(path.join(repoDir, "newdir", "leaf.ts")), { timeout: 10_000 })
      .toBe(true);
  });

  test("Escape cancels the inline input without creating a file", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("seed.ts", "seed");
    git.stageAll();
    git.commit("seed");

    const session = await setupTask(
      testPage,
      apiClient,
      seedData,
      "ft-create-cancel",
      "FT Create Cancel",
    );

    await expect(session.fileTreeNode("seed.ts")).toBeVisible({ timeout: 15_000 });

    const input = await startCreateAtRoot(testPage);
    await input.fill("ghost.ts");
    await input.press("Escape");

    await expect(testPage.getByPlaceholder("filename...")).toHaveCount(0, { timeout: 5_000 });
    await expect(session.fileTreeNode("ghost.ts")).toHaveCount(0);
    expect(fs.existsSync(path.join(repoDir, "ghost.ts"))).toBe(false);
  });
});
