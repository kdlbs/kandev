import { test, expect } from "../fixtures/test-base";
import path from "node:path";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../helpers/git-helper";

test.describe("Changes Panel — Active-Tab Highlight", () => {
  test("clicking a file row highlights the row while its diff tab is active", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));

    const profile = await createStandardProfile(apiClient, "active-highlight");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Active Highlight Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Active Highlight Test");
    await session.clickTab("Changes");

    git.createFile("active-a.ts", "a");
    git.createFile("active-b.ts", "b");

    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    const fileA = session.changesFileRow("active-a.ts");
    const fileB = session.changesFileRow("active-b.ts");
    await expect(fileA).toBeVisible({ timeout: 15_000 });
    await expect(fileB).toBeVisible({ timeout: 15_000 });

    // Both start inactive.
    await expect(fileA).toHaveAttribute("data-active", "false");
    await expect(fileB).toHaveAttribute("data-active", "false");

    // Open file-a → row A becomes active.
    await fileA.click();
    await expect(fileA).toHaveAttribute("data-active", "true", { timeout: 5_000 });
    await expect(fileB).toHaveAttribute("data-active", "false");
    await expect(session.changesActiveRows()).toHaveCount(1);

    // Switch to file-b → highlight follows.
    await fileB.click();
    await expect(fileB).toHaveAttribute("data-active", "true", { timeout: 5_000 });
    await expect(fileA).toHaveAttribute("data-active", "false");
    await expect(session.changesActiveRows()).toHaveCount(1);

    // Close the diff preview → nothing highlighted.
    await session.closeFileDiffPreview();
    await expect(fileA).toHaveAttribute("data-active", "false", { timeout: 5_000 });
    await expect(fileB).toHaveAttribute("data-active", "false");
    await expect(session.changesActiveRows()).toHaveCount(0);
  });

  test("highlight follows a file when it moves from unstaged to staged", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));

    const profile = await createStandardProfile(apiClient, "active-highlight-staged");
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Active Highlight Staged Test",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const session = await openTaskSession(testPage, "Active Highlight Staged Test");
    await session.clickTab("Changes");

    git.createFile("staged-active.ts", "a");

    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    const row = session.changesFileRow("staged-active.ts");
    await expect(row).toBeVisible({ timeout: 15_000 });

    await row.click();
    await expect(row).toHaveAttribute("data-active", "true", { timeout: 5_000 });

    // Stage the file via the existing per-file stage button; row re-renders in
    // the staged section but must keep data-active="true" since the diff tab
    // for the same path is still the active panel.
    git.stageFile("staged-active.ts");

    const stagedList = session.changes.getByTestId("staged-file-list");
    await expect(stagedList.getByText("staged-active.ts")).toBeVisible({ timeout: 15_000 });

    // The active row should now live inside the staged list.
    const stagedRow = stagedList.locator(`[data-changes-file="staged-active.ts"]`);
    await expect(stagedRow).toHaveAttribute("data-active", "true", { timeout: 5_000 });
  });
});
