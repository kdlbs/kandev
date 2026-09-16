import { type Page, type Locator } from "@playwright/test";
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

  const session = await openTaskSession(testPage, taskTitle);
  await session.clickTab("Files");
  return session;
}

async function dispatchHtmlDnd(testPage: Page, source: Locator, target: Locator) {
  // Make sure both are attached and visible.
  await source.scrollIntoViewIfNeeded();
  await target.scrollIntoViewIfNeeded();
  const sourceHandle = await source.elementHandle();
  const targetHandle = await target.elementHandle();
  if (!sourceHandle || !targetHandle) throw new Error("DnD: source/target missing");
  // Real browsers reuse the same DataTransfer across dragstart -> drop. We
  // do the whole sequence inside one evaluate() so the instance is shared.
  // dragover gets explicit preventDefault() because that's the contract
  // React's onDragOver fulfils when the drop is valid - without it, the
  // browser would interpret the drop as a navigation attempt for any
  // text-typed data and unload the page.
  await testPage.evaluate(
    ([src, dst]) => {
      const dt = new DataTransfer();
      const fireOn = (el: Element, type: string) => {
        const ev = new DragEvent(type, {
          bubbles: true,
          cancelable: true,
          composed: true,
          dataTransfer: dt,
        });
        el.dispatchEvent(ev);
        return ev;
      };
      fireOn(src, "dragstart");
      fireOn(dst, "dragenter");
      fireOn(dst, "dragover");
      fireOn(dst, "drop");
      // Source may have been removed from the DOM by the optimistic move -
      // guard the dragend call.
      if (src.isConnected) fireOn(src, "dragend");
    },
    [sourceHandle, targetHandle] as const,
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

    const file = await session.fileTree.waitForFileTreeNode("movable.ts");
    const folder = await session.fileTree.waitForFileTreeNode("target-dir");

    await dispatchHtmlDnd(testPage, file, folder);

    // The file is removed from the root immediately (optimistic update).
    await expect(session.fileTreeNode("movable.ts")).toHaveCount(0, { timeout: 10_000 });
    // Expand the target folder to verify the moved child landed inside.
    // moveNodesInTree does not auto-expand the drop target.
    await folder.click();
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

    const folder = await session.fileTree.waitForFileTreeNode("selfdir");

    // Drop onto self: handleDragOver short-circuits via isDropInvalid so
    // preventDefault is never called, which means the browser would never
    // fire drop in real usage. Dispatching events directly bypasses that
    // guard, but the drop handler also calls isDropInvalid and bails.
    await dispatchHtmlDnd(testPage, folder, folder);

    // Tree is unchanged: folder is still at root with its original child.
    await expect(session.fileTreeNode("selfdir")).toBeVisible({ timeout: 5_000 });
    // Expand and confirm the child is still there.
    await folder.click();
    await expect(session.fileTreeNode("selfdir/leaf.ts")).toBeVisible({ timeout: 10_000 });

    // Disk untouched - no self-nested directory created.
    expect(fs.existsSync(path.join(repoDir, "selfdir", "selfdir"))).toBe(false);
  });
});
