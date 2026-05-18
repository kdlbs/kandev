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

// File panel search lives in file-browser-search-header.tsx (input) +
// file-browser-hooks.ts (useFileBrowserSearch with 300ms debounce + WS call
// to searchWorkspaceFiles). Behaviour to lock in:
//   - Clicking the Search files toolbar button reveals the input
//   - Typing a query produces results that include matching files anywhere
//     in the tree (including inside non-expanded folders)
//   - Clearing the input goes back to the tree view
//   - Escape closes search

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

test.describe("File tree search", () => {
  test("search reveals matches inside collapsed folders", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("search-alpha.ts", "a");
    // Match lives inside a folder we never expand. Search must still find it.
    git.createFile("deep/nested/needle-target.ts", "needle");
    git.createFile("other-search/sibling.ts", "s");
    git.stageAll();
    git.commit("seed search");

    const session = await setupTask(
      testPage,
      apiClient,
      seedData,
      "ft-search-collapsed",
      "FT Search Collapsed",
    );

    await expect(session.fileTreeNode("search-alpha.ts")).toBeVisible({ timeout: 15_000 });
    // Sanity: target file's tree row is not in the DOM (folder is collapsed).
    await expect(session.fileTreeNode("deep/nested/needle-target.ts")).toHaveCount(0);

    await testPage.getByRole("button", { name: "Search files" }).click();
    const input = testPage.getByPlaceholder("Search files...");
    await expect(input).toBeVisible({ timeout: 5_000 });
    await input.fill("needle-target");

    // Search panel shows the matching path even though the parent folders
    // are not expanded in the tree.
    const match = session.files.getByText("needle-target.ts", { exact: false });
    await expect(match).toBeVisible({ timeout: 5_000 });
    // The folder path should appear next to the name.
    await expect(session.files.getByText("deep/nested/", { exact: false })).toBeVisible();
  });

  test("clearing the search input restores the tree view", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("clear-alpha.ts", "a");
    git.createFile("clear-bravo.ts", "b");
    git.stageAll();
    git.commit("seed clear");

    const session = await setupTask(
      testPage,
      apiClient,
      seedData,
      "ft-search-clear",
      "FT Search Clear",
    );

    await expect(session.fileTreeNode("clear-alpha.ts")).toBeVisible({ timeout: 15_000 });
    await testPage.getByRole("button", { name: "Search files" }).click();
    const input = testPage.getByPlaceholder("Search files...");
    await input.fill("clear-alpha");
    // Wait for the debounced search and the filtered list to settle.
    await expect(session.files.getByText("clear-alpha.ts", { exact: false })).toBeVisible({
      timeout: 5_000,
    });

    // Clear input - tree comes back, both files visible again.
    await input.fill("");
    await expect(session.fileTreeNode("clear-alpha.ts")).toBeVisible({ timeout: 5_000 });
    await expect(session.fileTreeNode("clear-bravo.ts")).toBeVisible({ timeout: 5_000 });
  });

  test("Escape closes the search header and returns to the tree", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("escape-target.ts", "e");
    git.stageAll();
    git.commit("seed escape");

    const session = await setupTask(
      testPage,
      apiClient,
      seedData,
      "ft-search-escape",
      "FT Search Escape",
    );

    await expect(session.fileTreeNode("escape-target.ts")).toBeVisible({ timeout: 15_000 });
    await testPage.getByRole("button", { name: "Search files" }).click();
    const input = testPage.getByPlaceholder("Search files...");
    await expect(input).toBeVisible({ timeout: 5_000 });
    await input.press("Escape");

    await expect(input).toHaveCount(0, { timeout: 5_000 });
    // Toolbar comes back - the search button is once again accessible.
    await expect(testPage.getByRole("button", { name: "Search files" })).toBeVisible();
  });
});
