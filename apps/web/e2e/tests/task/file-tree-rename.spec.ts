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

// Inline rename lives in file-context-menu.tsx (useFileRename + TreeNodeName).
// Entry points (today, in product code):
//   - Right-click -> "Rename" menu item
//   - The input is given focus 150ms after isRenaming=true and blur-commit is
//     gated by a 400ms ref so the initial focus race doesn't fire onBlur.
// Commit on Enter, cancel on Escape, commit on blur.
// We test the user-visible flow only (no direct DOM hacks), so the 400ms
// blur gate is exercised implicitly.

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

async function startRenameViaContextMenu(testPage: Page, node: ReturnType<Page["locator"]>) {
  await node.click({ button: "right" });
  const renameItem = testPage.getByRole("menuitem", { name: "Rename" });
  await expect(renameItem).toBeVisible({ timeout: 5_000 });
  await renameItem.click();
  // The input is auto-focused ~150ms after isRenaming flips. The Input inside
  // the row is the only one with this className combo; locate via role.
  const input = node.getByRole("textbox");
  await expect(input).toBeVisible({ timeout: 2_000 });
  await expect(input).toBeFocused({ timeout: 2_000 });
  return input;
}

test.describe("File tree inline rename", () => {
  test("rename commits on Enter and the new file appears on disk", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("rename-me.ts", "hello");
    git.stageAll();
    git.commit("seed rename file");

    const session = await setupTask(testPage, apiClient, seedData, "ft-rename", "FT Rename Enter");

    const node = session.fileTreeNode("rename-me.ts");
    await expect(node).toBeVisible({ timeout: 15_000 });

    const input = await startRenameViaContextMenu(testPage, node);
    // Select-all then type the new name (the hook also calls .select() but
    // doing it explicitly is robust against focus-timing edge cases).
    await input.press("ControlOrMeta+A");
    await input.fill("renamed.ts");
    await input.press("Enter");

    // Old node disappears from the tree, new node appears.
    await expect(session.fileTreeNode("rename-me.ts")).toHaveCount(0, { timeout: 10_000 });
    await expect(session.fileTreeNode("renamed.ts")).toBeVisible({ timeout: 10_000 });

    // And the rename hit the file system.
    await expect
      .poll(() => fs.existsSync(path.join(repoDir, "renamed.ts")), { timeout: 10_000 })
      .toBe(true);
    expect(fs.existsSync(path.join(repoDir, "rename-me.ts"))).toBe(false);
  });

  test("rename cancels on Escape and leaves the tree and disk untouched", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("keep-name.ts", "stay");
    git.stageAll();
    git.commit("seed keep file");

    const session = await setupTask(
      testPage,
      apiClient,
      seedData,
      "ft-rename-esc",
      "FT Rename Escape",
    );

    const node = session.fileTreeNode("keep-name.ts");
    await expect(node).toBeVisible({ timeout: 15_000 });

    const input = await startRenameViaContextMenu(testPage, node);
    await input.press("ControlOrMeta+A");
    await input.fill("nope.ts");
    await input.press("Escape");

    // No mutation - original node still present, no renamed node, disk unchanged.
    await expect(session.fileTreeNode("keep-name.ts")).toBeVisible({ timeout: 5_000 });
    await expect(session.fileTreeNode("nope.ts")).toHaveCount(0);
    expect(fs.existsSync(path.join(repoDir, "keep-name.ts"))).toBe(true);
    expect(fs.existsSync(path.join(repoDir, "nope.ts"))).toBe(false);
  });

  test("rename commits on blur after typing a new name", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("blur-original.ts", "blur");
    git.createFile("other.ts", "other");
    git.stageAll();
    git.commit("seed blur file");

    const session = await setupTask(
      testPage,
      apiClient,
      seedData,
      "ft-rename-blur",
      "FT Rename Blur",
    );

    const node = session.fileTreeNode("blur-original.ts");
    await expect(node).toBeVisible({ timeout: 15_000 });

    const input = await startRenameViaContextMenu(testPage, node);
    await input.press("ControlOrMeta+A");
    await input.fill("blur-final.ts");
    // The blur gate is ~400ms after isRenaming flips. Wait that out before
    // moving focus elsewhere so the blur handler actually fires the commit.
    await testPage.waitForTimeout(500);
    // Click another file to blur the input. The other node also belongs to
    // the tree, so we don't lose tree-container focus state.
    await session.fileTreeNode("other.ts").click();

    await expect(session.fileTreeNode("blur-final.ts")).toBeVisible({ timeout: 10_000 });
    await expect(session.fileTreeNode("blur-original.ts")).toHaveCount(0);
    await expect
      .poll(() => fs.existsSync(path.join(repoDir, "blur-final.ts")), { timeout: 10_000 })
      .toBe(true);
  });

  test("rename is a no-op when the name is unchanged", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("noop.ts", "noop");
    git.stageAll();
    git.commit("seed noop");

    const session = await setupTask(
      testPage,
      apiClient,
      seedData,
      "ft-rename-noop",
      "FT Rename NoOp",
    );

    const node = session.fileTreeNode("noop.ts");
    await expect(node).toBeVisible({ timeout: 15_000 });

    const input = await startRenameViaContextMenu(testPage, node);
    // Don't change anything, just press Enter.
    await input.press("Enter");

    await expect(session.fileTreeNode("noop.ts")).toBeVisible({ timeout: 5_000 });
    expect(fs.existsSync(path.join(repoDir, "noop.ts"))).toBe(true);
  });
});
