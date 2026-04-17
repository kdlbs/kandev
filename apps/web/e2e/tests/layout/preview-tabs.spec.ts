import path from "node:path";
import { expect, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../../helpers/git-helper";
import type { ApiClient } from "../../helpers/api-client";

// ---------------------------------------------------------------------------
// Setup helpers
// ---------------------------------------------------------------------------

type SeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  repositoryId: string;
};

async function setupFilesSession(args: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  taskTitle: string;
  profileName: string;
}): Promise<SessionPage> {
  const profile = await createStandardProfile(args.apiClient, args.profileName);
  await args.apiClient.createTaskWithAgent(args.seedData.workspaceId, args.taskTitle, profile.id, {
    description: "Preview tabs test",
    workflow_id: args.seedData.workflowId,
    workflow_step_id: args.seedData.startStepId,
    repository_ids: [args.seedData.repositoryId],
  });
  const session = await openTaskSession(args.testPage, args.taskTitle);
  await session.clickTab("Files");
  return session;
}

async function countTabsMatching(page: Page, text: string): Promise<number> {
  return page.locator(".dv-default-tab").filter({ hasText: text }).count();
}

const FILE_A = "alpha.ts";
const FILE_B = "beta.ts";

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("Editor preview tabs", () => {
  test("opening a second file replaces the preview file tab", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(FILE_A, "// alpha");
    git.createFile(FILE_B, "// beta");
    git.stageAll();
    git.commit("seed files");

    const session = await setupFilesSession({
      testPage,
      apiClient,
      seedData,
      taskTitle: "Preview replace file",
      profileName: "preview-replace-file",
    });

    // Open file A → preview tab shows alpha.ts
    await session.fileTreeNode(FILE_A).dblclick();
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 10_000 });
    expect(await countTabsMatching(testPage, FILE_A)).toBe(1);

    // Open file B → same tab, now shows beta.ts (alpha is gone)
    await session.fileTreeNode(FILE_B).dblclick();
    await expect(testPage.getByTestId("preview-tab-file-editor")).toHaveCount(1);
    expect(await countTabsMatching(testPage, FILE_B)).toBe(1);
    expect(await countTabsMatching(testPage, FILE_A)).toBe(0);
  });

  test("double-click promotes the preview tab to a pinned tab", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(FILE_A, "// alpha");
    git.createFile(FILE_B, "// beta");
    git.stageAll();
    git.commit("seed");

    const session = await setupFilesSession({
      testPage,
      apiClient,
      seedData,
      taskTitle: "Promote preview file",
      profileName: "promote-preview-file",
    });

    await session.fileTreeNode(FILE_A).dblclick();
    const previewTab = testPage.getByTestId("preview-tab-file-editor");
    await expect(previewTab).toBeVisible({ timeout: 10_000 });

    // Double-click the preview tab → promotes to pinned. Preview marker disappears.
    await previewTab.dblclick();
    await expect(previewTab).toHaveCount(0, { timeout: 5_000 });
    expect(await countTabsMatching(testPage, FILE_A)).toBe(1);

    // Open B as a new preview. A's pinned tab must still exist.
    await session.fileTreeNode(FILE_B).dblclick();
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 10_000 });
    expect(await countTabsMatching(testPage, FILE_A)).toBe(1);
    expect(await countTabsMatching(testPage, FILE_B)).toBe(1);
  });

  test("editing a preview file auto-pins it and a fresh preview opens for the next file", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(FILE_A, "content alpha");
    git.createFile(FILE_B, "content beta");
    git.stageAll();
    git.commit("seed");

    const session = await setupFilesSession({
      testPage,
      apiClient,
      seedData,
      taskTitle: "Auto pin on edit",
      profileName: "auto-pin-edit",
    });

    await session.fileTreeNode(FILE_A).dblclick();
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 10_000 });

    // Edit the file — type into the Monaco editor to mark it dirty.
    const editor = testPage.locator(".monaco-editor").first();
    await expect(editor).toBeVisible({ timeout: 10_000 });
    await editor.click();
    await testPage.keyboard.press("End");
    await testPage.keyboard.type("// edited");

    // After dirty, the preview should have been promoted: preview marker gone.
    await expect(testPage.getByTestId("preview-tab-file-editor")).toHaveCount(0, {
      timeout: 5_000,
    });
    expect(await countTabsMatching(testPage, FILE_A)).toBe(1);

    // Opening B should create a fresh preview, keeping A pinned.
    await session.fileTreeNode(FILE_B).dblclick();
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 10_000 });
    expect(await countTabsMatching(testPage, FILE_A)).toBe(1);
    expect(await countTabsMatching(testPage, FILE_B)).toBe(1);
  });

  test("middle-click closes a preview tab", async ({ testPage, apiClient, seedData, backend }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(FILE_A, "// alpha");
    git.stageAll();
    git.commit("seed");

    const session = await setupFilesSession({
      testPage,
      apiClient,
      seedData,
      taskTitle: "Middle click close",
      profileName: "middle-click-close",
    });

    await session.fileTreeNode(FILE_A).dblclick();
    const preview = testPage.getByTestId("preview-tab-file-editor");
    await expect(preview).toBeVisible({ timeout: 10_000 });

    await preview.click({ button: "middle" });
    await expect(preview).toHaveCount(0, { timeout: 5_000 });
    expect(await countTabsMatching(testPage, FILE_A)).toBe(0);
  });

  test("clicking different diffs in Changes replaces the single diff tab", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(FILE_A, "// base alpha");
    git.createFile(FILE_B, "// base beta");
    git.stageAll();
    git.commit("seed");
    // Modify both so they appear in Changes
    git.modifyFile(FILE_A, "// modified alpha");
    git.modifyFile(FILE_B, "// modified beta");

    const profile = await createStandardProfile(apiClient, "diff-preview");
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Diff preview replace", profile.id, {
      description: "diff preview",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const session = await openTaskSession(testPage, "Diff preview replace");
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });

    // Click file A in Changes → diff preview opens for A
    await session.changes.getByText(FILE_A).first().click();
    await expect(testPage.getByTestId("preview-tab-file-diff")).toBeVisible({ timeout: 10_000 });
    const alphaDiffTabs = testPage
      .locator(".dv-default-tab")
      .filter({ hasText: `Diff [${FILE_A}]` });
    await expect(alphaDiffTabs).toHaveCount(1);

    // Click file B → single diff tab, now for B
    await session.changes.getByText(FILE_B).first().click();
    const betaDiffTabs = testPage
      .locator(".dv-default-tab")
      .filter({ hasText: `Diff [${FILE_B}]` });
    await expect(betaDiffTabs).toHaveCount(1, { timeout: 10_000 });
    await expect(alphaDiffTabs).toHaveCount(0);
  });

  test("clicking different commits replaces the single commit-detail tab", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("commit-a.txt", "a1");
    git.stageFile("commit-a.txt");
    const sha1 = git.commit("commit one");
    git.createFile("commit-b.txt", "b1");
    git.stageFile("commit-b.txt");
    const sha2 = git.commit("commit two");

    const profile = await createStandardProfile(apiClient, "commit-preview");
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Commit preview replace",
      profile.id,
      {
        description: "commit preview",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const session = await openTaskSession(testPage, "Commit preview replace");
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 15_000 });

    const short1 = sha1.slice(0, 7);
    const short2 = sha2.slice(0, 7);

    // Click commit 1 → commit preview tab shows short1
    await testPage.getByTestId(`commit-row-${short1}`).click();
    await expect(testPage.getByTestId("preview-tab-commit-detail")).toBeVisible({
      timeout: 10_000,
    });
    const commitTabs = testPage
      .locator(".dv-default-tab")
      .filter({ hasText: new RegExp(`^(${short1}|${short2})$`) });
    await expect(commitTabs).toHaveCount(1);

    // Click commit 2 → same tab now shows short2
    await testPage.getByTestId(`commit-row-${short2}`).click();
    await expect(commitTabs).toHaveCount(1, { timeout: 10_000 });
    await expect(testPage.locator(".dv-default-tab").filter({ hasText: short2 })).toHaveCount(1);
    await expect(testPage.locator(".dv-default-tab").filter({ hasText: short1 })).toHaveCount(0);
  });

  test("opening a pinned item re-focuses the pinned tab and leaves preview alone", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(FILE_A, "// a");
    git.createFile(FILE_B, "// b");
    git.stageAll();
    git.commit("seed");

    const session = await setupFilesSession({
      testPage,
      apiClient,
      seedData,
      taskTitle: "Pinned focus",
      profileName: "pinned-focus",
    });

    // Open A → preview, pin it
    await session.fileTreeNode(FILE_A).dblclick();
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 10_000 });
    await testPage.getByTestId("preview-tab-file-editor").dblclick();
    await expect(testPage.getByTestId("preview-tab-file-editor")).toHaveCount(0);

    // Open B → preview
    await session.fileTreeNode(FILE_B).dblclick();
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 10_000 });

    // Click A in the tree again → focuses A's pinned tab. B preview survives.
    await session.fileTreeNode(FILE_A).dblclick();
    expect(await countTabsMatching(testPage, FILE_A)).toBe(1);
    expect(await countTabsMatching(testPage, FILE_B)).toBe(1);
    // Preview tab should still be the B tab.
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible();
  });
});
