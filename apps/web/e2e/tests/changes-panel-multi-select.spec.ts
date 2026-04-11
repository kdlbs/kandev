import { test, expect } from "../fixtures/test-base";
import path from "node:path";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../helpers/git-helper";

const MOD = process.platform === "darwin" ? ("Meta" as const) : ("Control" as const);

test.describe("Git Panel Multi-Select", () => {
  test("ctrl-click selects multiple unstaged files and shows bulk actions", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));

    const profile = await createStandardProfile(apiClient, "git-multi-select");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Multi-Select Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Multi-Select Test");
    await session.clickTab("Changes");

    // Create files AFTER session is open so WS detects the changes
    git.createFile("file-a.ts", "a");
    git.createFile("file-b.ts", "b");

    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    const fileA = session.changesFileRow("file-a.ts");
    const fileB = session.changesFileRow("file-b.ts");
    await expect(fileA).toBeVisible({ timeout: 15_000 });
    await expect(fileB).toBeVisible({ timeout: 15_000 });

    await fileA.click({ modifiers: [MOD] });
    await expect(fileA).toHaveAttribute("data-selected", "true");

    await fileB.click({ modifiers: [MOD] });
    await expect(fileA).toHaveAttribute("data-selected", "true");
    await expect(fileB).toHaveAttribute("data-selected", "true");

    const bulkBar = session.changesBulkActionBar("unstaged");
    await expect(bulkBar).toBeVisible({ timeout: 5_000 });
    await expect(session.changesBulkStageButton()).toBeVisible();
    await expect(session.changesBulkDiscardButton()).toBeVisible();
  });

  test("bulk stage moves selected files to staged section", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));

    const profile = await createStandardProfile(apiClient, "git-bulk-stage");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Bulk Stage Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Bulk Stage Test");
    await session.clickTab("Changes");

    git.createFile("stage-a.ts", "a");
    git.createFile("stage-b.ts", "b");

    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    const fileA = session.changesFileRow("stage-a.ts");
    const fileB = session.changesFileRow("stage-b.ts");
    await expect(fileA).toBeVisible({ timeout: 15_000 });
    await expect(fileB).toBeVisible({ timeout: 15_000 });

    await fileA.click({ modifiers: [MOD] });
    await fileB.click({ modifiers: [MOD] });

    await session.changesBulkStageButton().click();

    const stagedList = session.changes.getByTestId("staged-file-list");
    await expect(stagedList.getByText("stage-a.ts")).toBeVisible({ timeout: 15_000 });
    await expect(stagedList.getByText("stage-b.ts")).toBeVisible({ timeout: 15_000 });
  });

  test("escape clears selection in git panel", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));

    const profile = await createStandardProfile(apiClient, "git-escape");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Git Escape Test", profile.id, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const session = await openTaskSession(testPage, "Git Escape Test");
    await session.clickTab("Changes");

    git.createFile("esc-a.ts", "a");
    git.createFile("esc-b.ts", "b");

    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });
    const fileA = session.changesFileRow("esc-a.ts");
    await expect(fileA).toBeVisible({ timeout: 15_000 });

    await fileA.click({ modifiers: [MOD] });
    await expect(fileA).toHaveAttribute("data-selected", "true");

    const bulkBar = session.changesBulkActionBar("unstaged");
    await expect(bulkBar).toBeVisible({ timeout: 5_000 });

    const fileList = testPage.getByTestId("unstaged-file-list");
    await fileList.focus();
    await testPage.keyboard.press("Escape");

    await expect(session.changesSelectedRows()).toHaveCount(0, { timeout: 5_000 });
    await expect(bulkBar).not.toBeVisible({ timeout: 5_000 });
  });
});
