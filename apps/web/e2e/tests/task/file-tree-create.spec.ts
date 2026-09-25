import { type Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

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
  options: { profileName: string; taskTitle: string; requiredPath?: string },
) {
  const profile = await createStandardProfile(apiClient, options.profileName);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    options.taskTitle,
    profile.id,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (options.requiredPath) {
    let workspacePath = "";
    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.status ?? null, {
        timeout: 30_000,
        message: `Waiting for ${options.taskTitle} task environment to be ready`,
      })
      .toBe("ready");
    await expect
      .poll(
        async () => {
          const environment = await apiClient.getTaskEnvironment(task.id);
          const repositoryWorktree = environment?.repos?.find(
            (repository) => repository.repository_id === seedData.repositoryId,
          )?.worktree_path;
          // A task environment may expose the task root in workspace_path and
          // the repository checkout in repos[].worktree_path. The repository
          // id can be absent during the first materialization snapshot, so
          // include every advertised repository path until the exact fixture
          // file identifies the correct checkout.
          const candidatePaths = [
            repositoryWorktree,
            ...(environment?.repos ?? []).map((repository) => repository.worktree_path),
            environment?.workspace_path,
            environment?.worktree_path,
          ].filter(
            (candidate, index, paths): candidate is string =>
              Boolean(candidate) && paths.indexOf(candidate) === index,
          );
          workspacePath =
            candidatePaths.find((candidate) =>
              fs.existsSync(path.join(candidate, options.requiredPath!)),
            ) ?? "";
          return workspacePath !== "";
        },
        {
          timeout: 90_000,
          message: `Waiting for ${options.requiredPath} in the ${options.taskTitle} worktree`,
        },
      )
      .toBe(true);
  }

  // The task API is authoritative here. Direct navigation avoids a Kanban
  // card being replaced while the task snapshot is still settling.
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.clickTab("Files");
  return session;
}

async function startCreateAtRoot(testPage: Page) {
  const btn = testPage.getByRole("button", { name: "New file" });
  if (await btn.isVisible().catch(() => false)) {
    await btn.click();
  } else {
    const createMenu = testPage.getByTestId("files-create-menu");
    await expect(createMenu).toBeVisible({ timeout: 15_000 });
    await createMenu.click();
    const menuItem = testPage.getByRole("menuitem", { name: "New file" });
    await expect(menuItem).toBeVisible({ timeout: 5_000 });
    await menuItem.click();
    await expect(menuItem).toHaveCount(0);
  }
  const input = testPage.getByPlaceholder("filename...");
  await expect(input).toBeVisible({ timeout: 5_000 });
  await expect(input).toBeFocused({ timeout: 2_000 });
  return input;
}

test.describe("File tree create file", () => {
  test.describe.configure({ timeout: 180_000 });

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

    const session = await setupTask(testPage, apiClient, seedData, {
      profileName: "ft-create-root",
      taskTitle: "FT Create Root",
      requiredPath: "seed.ts",
    });

    await session.fileTree.waitForFileTreeNode("seed.ts", 45_000);

    const input = await startCreateAtRoot(testPage);
    await input.fill("brand-new.ts");
    await input.press("Enter");

    await expect(session.fileTreeNode("brand-new.ts")).toBeVisible({ timeout: 10_000 });
    await expect
      .poll(() => fs.existsSync(path.join(repoDir, "brand-new.ts")), { timeout: 10_000 })
      .toBe(true);
  });

  test("select-all stays in the new-file name input", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // @covers AC-UI-FILE-TREE-KEYBOARD-SCOPE-001.1
    // @covers AC-UI-FILE-TREE-KEYBOARD-SCOPE-001.2
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("select-all-alpha.ts", "alpha");
    git.createFile("select-all-beta.ts", "beta");
    git.stageAll();
    git.commit("seed select-all files");

    const session = await setupTask(testPage, apiClient, seedData, {
      profileName: "ft-create-select-all",
      taskTitle: "FT Create Select All",
      requiredPath: "select-all-alpha.ts",
    });
    await session.fileTree.waitForFileTreeNode("select-all-alpha.ts", 45_000);

    const input = await startCreateAtRoot(testPage);
    const draftName = "draft-name.ts";
    await input.fill(draftName);
    await input.press("ControlOrMeta+a");

    await expect
      .poll(async () => ({
        selection: await input.evaluate((element) => ({
          start: element.selectionStart,
          end: element.selectionEnd,
        })),
        selectedRows: await session.fileTreeSelectedNodes().count(),
      }))
      .toEqual({
        selection: { start: 0, end: draftName.length },
        selectedRows: 0,
      });
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

    const session = await setupTask(testPage, apiClient, seedData, {
      profileName: "ft-create-folder",
      taskTitle: "FT Create In Folder",
      requiredPath: "scope/existing.ts",
    });

    // Expand the folder so it becomes the "active folder" for handleStartCreate.
    const folder = await session.fileTree.waitForFileTreeNode("scope", 45_000);
    await folder.click();
    await expect(session.fileTreeNode("scope/existing.ts")).toBeVisible({ timeout: 10_000 });

    const input = await startCreateAtRoot(testPage);
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

    const session = await setupTask(testPage, apiClient, seedData, {
      profileName: "ft-create-implicit",
      taskTitle: "FT Create Implicit Folder",
      requiredPath: "seed.ts",
    });

    await session.fileTree.waitForFileTreeNode("seed.ts", 45_000);

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

    const session = await setupTask(testPage, apiClient, seedData, {
      profileName: "ft-create-cancel",
      taskTitle: "FT Create Cancel",
      requiredPath: "seed.ts",
    });

    await session.fileTree.waitForFileTreeNode("seed.ts", 45_000);

    const input = await startCreateAtRoot(testPage);
    await input.fill("ghost.ts");
    await input.press("Escape");

    await expect(testPage.getByPlaceholder("filename...")).toHaveCount(0, { timeout: 5_000 });
    await expect(session.fileTreeNode("ghost.ts")).toHaveCount(0);
    expect(fs.existsSync(path.join(repoDir, "ghost.ts"))).toBe(false);
  });
});
