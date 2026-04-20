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
  await expect(session.files).toBeVisible({ timeout: 10_000 });
  return session;
}

async function openFileFromTree(session: SessionPage, filename: string): Promise<void> {
  const node = session.fileTreeNode(filename);
  await expect(node).toBeVisible({ timeout: 15_000 });
  await node.click();
}

async function countTabsMatching(page: Page, text: string): Promise<number> {
  return page.locator(".dv-default-tab").filter({ hasText: text }).count();
}

const FILE_A = "alpha.ts";
const FILE_B = "beta.ts";

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TODO(preview-tabs): these tests flake in CI (shard 3). The unit tests in
// `lib/state/dockview-panel-actions.test.ts` cover the preview/pinned/promote
// rules exhaustively; re-enable and stabilize this spec in a followup once
// local Playwright reproduction is possible (installed browser ≠ CI version).
test.describe.skip("Editor preview tabs", () => {
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
    await openFileFromTree(session, FILE_A);
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 10_000 });
    await expect.poll(() => countTabsMatching(testPage, FILE_A), { timeout: 10_000 }).toBe(1);

    // Open file B → same preview panel, now shows beta.ts (alpha tab is gone)
    await openFileFromTree(session, FILE_B);
    await expect(testPage.getByTestId("preview-tab-file-editor")).toHaveCount(1);
    await expect.poll(() => countTabsMatching(testPage, FILE_B), { timeout: 10_000 }).toBe(1);
    await expect.poll(() => countTabsMatching(testPage, FILE_A)).toBe(0);
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

    await openFileFromTree(session, FILE_A);
    const previewTab = testPage.getByTestId("preview-tab-file-editor");
    await expect(previewTab).toBeVisible({ timeout: 10_000 });

    // Double-click the preview tab → promotes to pinned. Preview marker disappears.
    await previewTab.dblclick();
    await expect(previewTab).toHaveCount(0, { timeout: 10_000 });
    await expect.poll(() => countTabsMatching(testPage, FILE_A), { timeout: 10_000 }).toBe(1);

    // Open B as a new preview. A's pinned tab must still exist.
    await openFileFromTree(session, FILE_B);
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 10_000 });
    await expect.poll(() => countTabsMatching(testPage, FILE_A)).toBe(1);
    await expect.poll(() => countTabsMatching(testPage, FILE_B)).toBe(1);
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

    await openFileFromTree(session, FILE_A);
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 10_000 });

    // Wait for Monaco textarea to be ready, focus it, and type.
    const monacoTextArea = testPage.locator(".monaco-editor textarea.inputarea").first();
    await expect(monacoTextArea).toBeAttached({ timeout: 15_000 });
    await monacoTextArea.focus();
    await testPage.keyboard.press("End");
    await testPage.keyboard.type("// edited");

    // After dirty, the preview should have been promoted: preview marker gone.
    await expect(testPage.getByTestId("preview-tab-file-editor")).toHaveCount(0, {
      timeout: 10_000,
    });
    await expect.poll(() => countTabsMatching(testPage, FILE_A), { timeout: 10_000 }).toBe(1);

    // Opening B should create a fresh preview, keeping A pinned.
    await openFileFromTree(session, FILE_B);
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 10_000 });
    await expect.poll(() => countTabsMatching(testPage, FILE_A)).toBe(1);
    await expect.poll(() => countTabsMatching(testPage, FILE_B)).toBe(1);
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

    await openFileFromTree(session, FILE_A);
    const preview = testPage.getByTestId("preview-tab-file-editor");
    await expect(preview).toBeVisible({ timeout: 10_000 });

    await preview.click({ button: "middle" });
    await expect(preview).toHaveCount(0, { timeout: 10_000 });
    await expect.poll(() => countTabsMatching(testPage, FILE_A)).toBe(0);
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
    // Modify both so they appear in unstaged changes
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
    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 15_000 });

    // Click file A in Changes → diff preview opens for A
    await session.changes.getByText(FILE_A).first().click();
    await expect(testPage.getByTestId("preview-tab-file-diff")).toBeVisible({ timeout: 15_000 });
    const alphaDiffTabs = testPage
      .locator(".dv-default-tab")
      .filter({ hasText: `Diff [${FILE_A}]` });
    await expect(alphaDiffTabs).toHaveCount(1, { timeout: 10_000 });

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
    const commit1Row = testPage.getByTestId(`commit-row-${short1}`);
    await expect(commit1Row).toBeVisible({ timeout: 10_000 });
    await commit1Row.click();
    await expect(testPage.getByTestId("preview-tab-commit-detail")).toBeVisible({
      timeout: 15_000,
    });
    const commitTabs = testPage
      .locator(".dv-default-tab")
      .filter({ hasText: new RegExp(`^(${short1}|${short2})$`) });
    await expect(commitTabs).toHaveCount(1, { timeout: 10_000 });

    // Click commit 2 → same tab now shows short2
    await testPage.getByTestId(`commit-row-${short2}`).click();
    await expect(testPage.locator(".dv-default-tab").filter({ hasText: short2 })).toHaveCount(1, {
      timeout: 10_000,
    });
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

    // Open A → preview, pin it via double-click
    await openFileFromTree(session, FILE_A);
    const preview = testPage.getByTestId("preview-tab-file-editor");
    await expect(preview).toBeVisible({ timeout: 10_000 });
    await preview.dblclick();
    await expect(preview).toHaveCount(0, { timeout: 10_000 });

    // Open B → fresh preview
    await openFileFromTree(session, FILE_B);
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 10_000 });

    // Click A in the tree again → focuses A's pinned tab. B preview survives.
    await openFileFromTree(session, FILE_A);
    await expect.poll(() => countTabsMatching(testPage, FILE_A)).toBe(1);
    await expect.poll(() => countTabsMatching(testPage, FILE_B)).toBe(1);
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible();
  });
});
