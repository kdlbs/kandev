import { type Page } from "@playwright/test";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../../helpers/git-helper";

// Children of a directory are loaded lazily when the directory is first expanded
// (see useFileBrowserHandlers.toggleExpand -> loadNodeChildren). The pre-refactor
// behaviour we need to lock in: children rows render *after* a directory click,
// they were not in the DOM before, and re-expanding doesn't refetch synchronously
// (children persist across collapse/expand once loaded).

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

test.describe("File tree lazy-load on expand", () => {
  test("children render only after directory is expanded", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("lazyfolder/child-a.ts", "a");
    git.createFile("lazyfolder/child-b.ts", "b");
    git.createFile("lazyfolder/nested/deep.ts", "deep");
    git.stageAll();
    git.commit("seed lazy folder");

    const session = await setupTask(testPage, apiClient, seedData, "ft-lazy-load", "FT Lazy Load");

    const folder = session.fileTreeNode("lazyfolder");
    await expect(folder).toBeVisible({ timeout: 15_000 });

    // Pre-expand: children should not be in the DOM yet.
    await expect(session.fileTreeNode("lazyfolder/child-a.ts")).toHaveCount(0);
    await expect(session.fileTreeNode("lazyfolder/child-b.ts")).toHaveCount(0);
    await expect(session.fileTreeNode("lazyfolder/nested")).toHaveCount(0);

    // Expand the folder.
    await folder.click();

    // After expand: direct children render. Nested subfolder appears but its
    // own children are not loaded until that subfolder is also expanded.
    await expect(session.fileTreeNode("lazyfolder/child-a.ts")).toBeVisible({ timeout: 10_000 });
    await expect(session.fileTreeNode("lazyfolder/child-b.ts")).toBeVisible({ timeout: 10_000 });
    await expect(session.fileTreeNode("lazyfolder/nested")).toBeVisible({ timeout: 10_000 });
    await expect(session.fileTreeNode("lazyfolder/nested/deep.ts")).toHaveCount(0);

    // Expand nested -> its child loads.
    await session.fileTreeNode("lazyfolder/nested").click();
    await expect(session.fileTreeNode("lazyfolder/nested/deep.ts")).toBeVisible({
      timeout: 10_000,
    });
  });

  test("collapsing then re-expanding keeps children without refetch", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("keepfolder/keep-a.ts", "a");
    git.createFile("keepfolder/keep-b.ts", "b");
    git.stageAll();
    git.commit("seed keep folder");

    const session = await setupTask(testPage, apiClient, seedData, "ft-lazy-keep", "FT Lazy Keep");

    const folder = session.fileTreeNode("keepfolder");
    await expect(folder).toBeVisible({ timeout: 15_000 });

    // Expand
    await folder.click();
    await expect(session.fileTreeNode("keepfolder/keep-a.ts")).toBeVisible({ timeout: 10_000 });

    // Collapse - children removed from DOM
    await folder.click();
    await expect(session.fileTreeNode("keepfolder/keep-a.ts")).toHaveCount(0, { timeout: 5_000 });

    // Re-expand - children come back quickly (already cached in tree state).
    await folder.click();
    await expect(session.fileTreeNode("keepfolder/keep-a.ts")).toBeVisible({ timeout: 5_000 });
    await expect(session.fileTreeNode("keepfolder/keep-b.ts")).toBeVisible({ timeout: 5_000 });
  });
});
