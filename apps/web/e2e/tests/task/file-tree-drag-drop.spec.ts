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

function visibleFileTreeNode(testPage: Page, nodePath: string): Locator {
  return testPage
    .locator(
      `[data-testid="files-panel"]:visible [data-testid="file-tree"]:visible [data-testid="file-tree-node"][data-path="${nodePath}"]`,
    )
    .first();
}

async function focusVisibleFilesTree(testPage: Page): Promise<boolean> {
  const tree = testPage.locator(
    '[data-testid="files-panel"]:visible [data-testid="file-tree"]:visible',
  );
  if ((await tree.count()) === 1) return true;

  await testPage
    .locator(".dv-default-tab:visible")
    .filter({ hasText: /^Files$/ })
    .first()
    .click({ timeout: 2_000 })
    .catch(() => undefined);

  return (await tree.count()) === 1;
}

async function expectSingleVisibleFileTree(testPage: Page) {
  await expect
    .poll(() => focusVisibleFilesTree(testPage), {
      timeout: 15_000,
      message: "Expected exactly one visible Files tree",
    })
    .toBe(true);
}

async function expandVisibleFolder(testPage: Page, folderPath: string, childPath: string) {
  await expect
    .poll(
      async () => {
        if (!(await focusVisibleFilesTree(testPage))) return false;
        if ((await visibleFileTreeNode(testPage, childPath).count()) > 0) return true;

        const folder = visibleFileTreeNode(testPage, folderPath);
        if ((await folder.count()) === 0) return false;

        await folder.scrollIntoViewIfNeeded().catch(() => undefined);
        const isExpanded =
          (await folder.getAttribute("data-expanded").catch(() => null)) === "true";
        if (!isExpanded) {
          await folder.click({ timeout: 2_000 }).catch(() => undefined);
        }
        return (await visibleFileTreeNode(testPage, childPath).count()) > 0;
      },
      {
        timeout: 30_000,
        message: `Expected ${folderPath} to expand and reveal ${childPath}`,
      },
    )
    .toBe(true);
}

test.describe("File tree drag and drop", () => {
  test.describe.configure({ timeout: 90_000 });

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

    await setupTask(testPage, apiClient, seedData, "ft-dnd-move", "FT DnD Move");
    await expectSingleVisibleFileTree(testPage);

    const file = visibleFileTreeNode(testPage, "movable.ts");
    const folder = visibleFileTreeNode(testPage, "target-dir");
    await expect(file).toBeVisible({ timeout: 15_000 });
    await expect(folder).toBeVisible({ timeout: 15_000 });

    await dispatchHtmlDnd(testPage, file, folder);

    // The file is removed from the root immediately (optimistic update).
    await expect
      .poll(
        async () => {
          if (!(await focusVisibleFilesTree(testPage))) return -1;
          return visibleFileTreeNode(testPage, "movable.ts").count();
        },
        { timeout: 20_000 },
      )
      .toBe(0);

    await expect
      .poll(() => fs.existsSync(path.join(repoDir, "target-dir", "movable.ts")), {
        timeout: 20_000,
      })
      .toBe(true);
    expect(fs.existsSync(path.join(repoDir, "movable.ts"))).toBe(false);

    // Re-acquire the folder after the optimistic tree update settles, then expand
    // it to verify the moved child landed inside. moveNodesInTree does not
    // auto-expand the drop target. Git status updates can focus Changes during
    // this window, so helpers below keep bringing the visible Files tree back.
    await expectSingleVisibleFileTree(testPage);
    const movedFolder = visibleFileTreeNode(testPage, "target-dir");
    await expect(movedFolder).toBeVisible({ timeout: 20_000 });
    await expandVisibleFolder(testPage, "target-dir", "target-dir/movable.ts");
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

    await setupTask(testPage, apiClient, seedData, "ft-dnd-self", "FT DnD Self Reject");
    await expectSingleVisibleFileTree(testPage);

    const folder = visibleFileTreeNode(testPage, "selfdir");
    await expect(folder).toBeVisible({ timeout: 15_000 });

    // Drop onto self: handleDragOver short-circuits via isDropInvalid so
    // preventDefault is never called, which means the browser would never
    // fire drop in real usage. Dispatching events directly bypasses that
    // guard, but the drop handler also calls isDropInvalid and bails.
    await dispatchHtmlDnd(testPage, folder, folder);

    // Tree is unchanged: folder is still at root with its original child.
    await expect(visibleFileTreeNode(testPage, "selfdir")).toBeVisible({ timeout: 5_000 });
    // Expand and confirm the child is still there.
    await expandVisibleFolder(testPage, "selfdir", "selfdir/leaf.ts");

    // Disk untouched - no self-nested directory created.
    expect(fs.existsSync(path.join(repoDir, "selfdir", "selfdir"))).toBe(false);
  });
});
