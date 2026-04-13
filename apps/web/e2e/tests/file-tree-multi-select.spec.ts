import { test, expect } from "../fixtures/test-base";
import path from "node:path";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../helpers/git-helper";

const MOD = process.platform === "darwin" ? ("Meta" as const) : ("Control" as const);

async function setupFileTreeTest(
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

test.describe("File Tree Multi-Select", () => {
  // --- Selection Basics ---

  test("ctrl-click selects a file without opening it", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("alpha.ts", "a");
    git.createFile("beta.ts", "b");
    git.stageAll();
    git.commit("add files");

    const session = await setupFileTreeTest(
      testPage,
      apiClient,
      seedData,
      "ft-select-1",
      "FT Select File",
    );
    const alpha = session.fileTreeNode("alpha.ts");
    await expect(alpha).toBeVisible({ timeout: 15_000 });

    await alpha.click({ modifiers: [MOD] });
    await expect(alpha).toHaveAttribute("data-selected", "true");
  });

  test("ctrl-click toggles file out of selection", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("toggle-a.ts", "a");
    git.stageAll();
    git.commit("add toggle file");

    const session = await setupFileTreeTest(
      testPage,
      apiClient,
      seedData,
      "ft-toggle",
      "FT Toggle File",
    );
    const file = session.fileTreeNode("toggle-a.ts");
    await expect(file).toBeVisible({ timeout: 15_000 });

    await file.click({ modifiers: [MOD] });
    await expect(file).toHaveAttribute("data-selected", "true");

    await file.click({ modifiers: [MOD] });
    await expect(file).toHaveAttribute("data-selected", "false");
  });

  test("ctrl-click selects multiple files", async ({ testPage, apiClient, seedData, backend }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("multi-a.ts", "a");
    git.createFile("multi-b.ts", "b");
    git.createFile("multi-c.ts", "c");
    git.stageAll();
    git.commit("add multi files");

    const session = await setupFileTreeTest(
      testPage,
      apiClient,
      seedData,
      "ft-multi",
      "FT Multi Select",
    );
    const a = session.fileTreeNode("multi-a.ts");
    const b = session.fileTreeNode("multi-b.ts");
    await expect(a).toBeVisible({ timeout: 15_000 });

    await a.click({ modifiers: [MOD] });
    await b.click({ modifiers: [MOD] });
    await expect(a).toHaveAttribute("data-selected", "true");
    await expect(b).toHaveAttribute("data-selected", "true");
  });

  test("plain click clears selection", async ({ testPage, apiClient, seedData, backend }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("clear-a.ts", "a");
    git.createFile("clear-b.ts", "b");
    git.stageAll();
    git.commit("add clear files");

    const session = await setupFileTreeTest(
      testPage,
      apiClient,
      seedData,
      "ft-clear",
      "FT Plain Click Clear",
    );
    const a = session.fileTreeNode("clear-a.ts");
    const b = session.fileTreeNode("clear-b.ts");
    await expect(a).toBeVisible({ timeout: 15_000 });

    await a.click({ modifiers: [MOD] });
    await b.click({ modifiers: [MOD] });
    await expect(session.fileTreeSelectedNodes()).toHaveCount(2);

    await a.click();
    await expect(session.fileTreeSelectedNodes()).toHaveCount(0);
  });

  // --- Shift-Click Range ---

  test("shift-click selects range after plain click anchor", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("r-a.ts", "a");
    git.createFile("r-b.ts", "b");
    git.createFile("r-c.ts", "c");
    git.createFile("r-d.ts", "d");
    git.stageAll();
    git.commit("add range files");

    const session = await setupFileTreeTest(
      testPage,
      apiClient,
      seedData,
      "ft-range",
      "FT Shift Range",
    );
    const a = session.fileTreeNode("r-a.ts");
    const d = session.fileTreeNode("r-d.ts");
    await expect(a).toBeVisible({ timeout: 15_000 });
    await expect(d).toBeVisible({ timeout: 15_000 });

    await a.click();
    await d.click({ modifiers: ["Shift"] });

    await expect(session.fileTreeNode("r-a.ts")).toHaveAttribute("data-selected", "true");
    await expect(session.fileTreeNode("r-b.ts")).toHaveAttribute("data-selected", "true");
    await expect(session.fileTreeNode("r-c.ts")).toHaveAttribute("data-selected", "true");
    await expect(session.fileTreeNode("r-d.ts")).toHaveAttribute("data-selected", "true");
  });

  test("shift-click selects range after ctrl-click anchor", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("s-a.ts", "a");
    git.createFile("s-b.ts", "b");
    git.createFile("s-c.ts", "c");
    git.stageAll();
    git.commit("add shift range files");

    const session = await setupFileTreeTest(
      testPage,
      apiClient,
      seedData,
      "ft-shift-anchor",
      "FT Shift Anchor",
    );
    const b = session.fileTreeNode("s-b.ts");
    const c = session.fileTreeNode("s-c.ts");
    await expect(b).toBeVisible({ timeout: 15_000 });

    await b.click({ modifiers: [MOD] });
    await expect(b).toHaveAttribute("data-selected", "true");

    await c.click({ modifiers: ["Shift"] });
    await expect(session.fileTreeNode("s-b.ts")).toHaveAttribute("data-selected", "true");
    await expect(session.fileTreeNode("s-c.ts")).toHaveAttribute("data-selected", "true");
  });

  // --- Directory Selection ---

  test("ctrl-click selects a directory without expanding it", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("mydir/nested.ts", "n");
    git.stageAll();
    git.commit("add dir");

    const session = await setupFileTreeTest(
      testPage,
      apiClient,
      seedData,
      "ft-dir-select",
      "FT Dir Select",
    );
    const dir = session.fileTreeNode("mydir");
    await expect(dir).toBeVisible({ timeout: 15_000 });

    await dir.click({ modifiers: [MOD] });
    await expect(dir).toHaveAttribute("data-selected", "true");
  });

  test("ctrl-click selects file inside expanded directory", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("srcdir/inner.ts", "inner");
    git.stageAll();
    git.commit("add inner file");

    const session = await setupFileTreeTest(
      testPage,
      apiClient,
      seedData,
      "ft-nested-select",
      "FT Nested Select",
    );
    const dir = session.fileTreeNode("srcdir");
    await expect(dir).toBeVisible({ timeout: 15_000 });
    await dir.click();

    const inner = session.fileTreeNode("srcdir/inner.ts");
    await expect(inner).toBeVisible({ timeout: 10_000 });
    await inner.click({ modifiers: [MOD] });
    await expect(inner).toHaveAttribute("data-selected", "true");
    await expect(dir).toHaveAttribute("data-selected", "false");
  });

  // --- Right-Click Context Menu ---

  test("right-click single file shows context menu with delete", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("ctx-file.ts", "ctx");
    git.stageAll();
    git.commit("add ctx file");

    const session = await setupFileTreeTest(
      testPage,
      apiClient,
      seedData,
      "ft-ctx-menu",
      "FT Context Menu",
    );
    const file = session.fileTreeNode("ctx-file.ts");
    await expect(file).toBeVisible({ timeout: 15_000 });

    await file.click({ button: "right" });
    await expect(testPage.getByRole("menuitem", { name: "Delete" })).toBeVisible({
      timeout: 5_000,
    });
  });

  test("right-click on multi-selection shows bulk delete", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("bulk-a.ts", "a");
    git.createFile("bulk-b.ts", "b");
    git.stageAll();
    git.commit("add bulk files");

    const session = await setupFileTreeTest(
      testPage,
      apiClient,
      seedData,
      "ft-bulk-ctx",
      "FT Bulk Context",
    );
    const a = session.fileTreeNode("bulk-a.ts");
    const b = session.fileTreeNode("bulk-b.ts");
    await expect(a).toBeVisible({ timeout: 15_000 });

    await a.click({ modifiers: [MOD] });
    await b.click({ modifiers: [MOD] });

    await a.click({ button: "right" });
    const deleteItem = testPage.getByRole("menuitem", { name: "Delete 2 items" });
    await expect(deleteItem).toBeVisible({ timeout: 5_000 });

    // Click the delete menu item and confirm dialog
    await deleteItem.click();
    const confirmBtn = testPage.getByRole("button", { name: "Delete" });
    await expect(confirmBtn).toBeVisible({ timeout: 5_000 });
    await confirmBtn.click();

    // Both files should be removed from the tree
    await expect(a).not.toBeVisible({ timeout: 10_000 });
    await expect(b).not.toBeVisible({ timeout: 10_000 });
  });

  // --- Click Outside ---

  test("clicking outside tree clears selection", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("outside-a.ts", "a");
    git.stageAll();
    git.commit("add outside file");

    const session = await setupFileTreeTest(
      testPage,
      apiClient,
      seedData,
      "ft-click-outside",
      "FT Click Outside",
    );
    const file = session.fileTreeNode("outside-a.ts");
    await expect(file).toBeVisible({ timeout: 15_000 });

    await file.click({ modifiers: [MOD] });
    await expect(file).toHaveAttribute("data-selected", "true");

    // Click empty area in the files panel header area
    await session.files.click({ position: { x: 5, y: 5 } });
    await expect(session.fileTreeSelectedNodes()).toHaveCount(0, { timeout: 5_000 });
  });

  // --- Keyboard ---

  test("escape clears selection", async ({ testPage, apiClient, seedData, backend }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile("esc-file.ts", "e");
    git.stageAll();
    git.commit("add esc file");

    const session = await setupFileTreeTest(
      testPage,
      apiClient,
      seedData,
      "ft-escape",
      "FT Escape",
    );
    const file = session.fileTreeNode("esc-file.ts");
    await expect(file).toBeVisible({ timeout: 15_000 });

    await file.click({ modifiers: [MOD] });
    await expect(file).toHaveAttribute("data-selected", "true");

    await file.press("Escape");
    await expect(session.fileTreeSelectedNodes()).toHaveCount(0, { timeout: 5_000 });
  });
});
