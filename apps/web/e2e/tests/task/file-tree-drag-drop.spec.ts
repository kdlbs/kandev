import { type Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";

// DnD in file-browser.tsx uses native HTML5 drag events (dragstart, dragover,
// drop) keyed off React's onDragStart/Over/Drop. Playwright's locator.dragTo()
// does not trigger native HTML5 DnD reliably in Chromium - the drop target
// must see dragover with preventDefault() and a drop event with the same
// DataTransfer that was set in dragstart.
//
// We dispatch the events manually via page.evaluate(), constructing a shared
// DataTransfer for the dragstart -> drop sequence. This is the established
// workaround for testing HTML5 DnD in Playwright and mirrors what the user
// would do.

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

  // The task API returns before local workspace preparation finishes. Wait
  // for the environment's durable ready state before the first tree request;
  // otherwise the tree can legitimately snapshot the repository while the
  // agent session is still being attached to it.
  let workspacePath = "";
  await expect
    .poll(async () => (await apiClient.getTaskEnvironment(task.id))?.status ?? null, {
      timeout: 30_000,
      message: `Waiting for ${taskTitle} task environment to be ready`,
    })
    .toBe("ready");

  // Environment readiness and repository materialization are separate
  // transitions. Wait for the exact fixture file in the task worktree before
  // the first tree snapshot, otherwise a valid early tree can be retained
  // while the checkout is still being populated.
  await expect
    .poll(
      async () => {
        const environment = await apiClient.getTaskEnvironment(task.id);
        workspacePath = environment?.workspace_path ?? environment?.repos?.[0]?.worktree_path ?? "";
        return Boolean(workspacePath && fs.existsSync(path.join(workspacePath, requiredPath)));
      },
      { timeout: 60_000, message: `Waiting for ${requiredPath} in the ${taskTitle} worktree` },
    )
    .toBe(true);

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.clickTab("Files");
  return session;
}

async function dispatchHtmlDnd(testPage: Page, sourcePath: string, targetPath: string) {
  // Virtualized trees can unmount the source while the target is revealed.
  // Keep the browser DataTransfer on the page between the two scrolls so the
  // source and target do not need to be mounted at the same time.
  const source = testPage.locator(
    `[data-testid="file-tree-node"][data-path=${JSON.stringify(sourcePath)}]:visible`,
  );
  const target = testPage.locator(
    `[data-testid="file-tree-node"][data-path=${JSON.stringify(targetPath)}]:visible`,
  );
  await expect(source).toBeVisible({ timeout: 30_000 });
  await source.scrollIntoViewIfNeeded();
  await testPage.evaluate((nodePath) => {
    const row = Array.from(document.querySelectorAll('[data-testid="file-tree-node"]')).find(
      (element) =>
        element.getAttribute("data-path") === nodePath &&
        element.getBoundingClientRect().width > 0 &&
        element.getBoundingClientRect().height > 0,
    );
    if (!row) throw new Error(`DnD source is not mounted: ${nodePath}`);
    const dataTransfer = new DataTransfer();
    const event = new DragEvent("dragstart", {
      bubbles: true,
      cancelable: true,
      composed: true,
      dataTransfer,
    });
    row.dispatchEvent(event);
    Object.defineProperty(window, "__kandevE2eDataTransfer", {
      configurable: true,
      value: dataTransfer,
    });
  }, sourcePath);

  await expect(target).toBeVisible({ timeout: 30_000 });
  await target.scrollIntoViewIfNeeded();
  await testPage.evaluate(
    ({ sourcePath, targetPath: nodePath }) => {
      const row = Array.from(document.querySelectorAll('[data-testid="file-tree-node"]')).find(
        (element) =>
          element.getAttribute("data-path") === nodePath &&
          element.getBoundingClientRect().width > 0 &&
          element.getBoundingClientRect().height > 0,
      );
      const dataTransfer = (window as Window & { __kandevE2eDataTransfer?: DataTransfer })
        .__kandevE2eDataTransfer;
      if (!row || !dataTransfer) throw new Error(`DnD target is not mounted: ${nodePath}`);
      const fireOn = (element: Element, type: string) => {
        element.dispatchEvent(
          new DragEvent(type, { bubbles: true, cancelable: true, composed: true, dataTransfer }),
        );
      };
      fireOn(row, "dragenter");
      fireOn(row, "dragover");
      fireOn(row, "drop");
      const source = Array.from(document.querySelectorAll('[data-testid="file-tree-node"]')).find(
        (element) => element.getAttribute("data-path") === sourcePath,
      );
      if (source) fireOn(source, "dragend");
      document.dispatchEvent(new DragEvent("dragend", { bubbles: true, dataTransfer }));
      delete (window as Window & { __kandevE2eDataTransfer?: DataTransfer })
        .__kandevE2eDataTransfer;
    },
    { sourcePath, targetPath },
  );
}

test.describe("File tree drag and drop", () => {
  test("drag a file into a folder moves it on disk and in the tree", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("movable.ts", "m");
    git.createFile("target-dir/keep.ts", "k");
    git.stageAll();
    git.commit("seed dnd");

    const session = await setupTask({
      testPage,
      apiClient,
      seedData,
      profileName: "ft-dnd-move",
      taskTitle: "FT DnD Move",
      requiredPath: "movable.ts",
    });

    await session.fileTree.waitForFileTreeNode("movable.ts");
    await session.fileTree.waitForFileTreeNode("target-dir");

    await dispatchHtmlDnd(testPage, "movable.ts", "target-dir");

    // The file is removed from the root immediately (optimistic update).
    await expect(session.fileTreeNode("movable.ts")).toHaveCount(0, { timeout: 10_000 });
    // Expand the target folder to verify the moved child landed inside.
    // moveNodesInTree does not auto-expand the drop target.
    await session.fileTreeNode("target-dir").click();
    await expect(session.fileTreeNode("target-dir/movable.ts")).toBeVisible({ timeout: 10_000 });

    await expect
      .poll(() => fs.existsSync(path.join(repoDir, "target-dir", "movable.ts")), {
        timeout: 10_000,
      })
      .toBe(true);
    expect(fs.existsSync(path.join(repoDir, "movable.ts"))).toBe(false);
  });

  test("drop is rejected when dragging a folder onto itself", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("selfdir/leaf.ts", "leaf");
    git.stageAll();
    git.commit("seed selfdir");

    const session = await setupTask({
      testPage,
      apiClient,
      seedData,
      profileName: "ft-dnd-self",
      taskTitle: "FT DnD Self Reject",
      requiredPath: "selfdir/leaf.ts",
    });

    await session.fileTree.waitForFileTreeNode("selfdir");

    // Drop onto self: handleDragOver short-circuits via isDropInvalid so
    // preventDefault is never called, which means the browser would never
    // fire drop in real usage. Dispatching events directly bypasses that
    // guard, but the drop handler also calls isDropInvalid and bails.
    await dispatchHtmlDnd(testPage, "selfdir", "selfdir");

    // Tree is unchanged: folder is still at root with its original child.
    await expect(session.fileTreeNode("selfdir")).toBeVisible({ timeout: 5_000 });
    // Expand and confirm the child is still there.
    await session.fileTreeNode("selfdir").click();
    await expect(session.fileTreeNode("selfdir/leaf.ts")).toBeVisible({ timeout: 10_000 });

    // Disk untouched - no self-nested directory created.
    expect(fs.existsSync(path.join(repoDir, "selfdir", "selfdir"))).toBe(false);
  });
});
