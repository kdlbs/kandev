import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";

const SET_ROW = "repository-set-row";
const EDITOR_NAME = "repository-set-editor-name";
const EDITOR_SAVE = "repository-set-editor-save";
const SECOND_REPO_NAME = "Settings Sets Target";

test.describe("Workspace repository sets settings", () => {
  test("creates, edits, and deletes a set without touching its repositories", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const dir = path.join(backend.tmpDir, "repos", "settings-repository-sets");
    const remoteDir = path.join(backend.tmpDir, "repos", "settings-repository-sets-origin.git");
    fs.mkdirSync(dir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync(`git init --bare -b main "${remoteDir}"`, { env: gitEnv });
    execSync("git init -b main", { cwd: dir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: dir, env: gitEnv });
    execSync("git branch develop", { cwd: dir, env: gitEnv });
    execSync(`git remote add origin "file://${remoteDir}"`, { cwd: dir, env: gitEnv });
    execSync("git push origin main", { cwd: dir, env: gitEnv });
    execSync("git update-ref refs/remotes/origin/main HEAD", { cwd: dir, env: gitEnv });
    const second = await apiClient.createRepository(seedData.workspaceId, dir, "main", {
      name: SECOND_REPO_NAME,
    });

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    await expect(testPage.getByTestId("repository-sets-empty")).toBeVisible();

    // Create a set holding both repositories.
    const setName = `Settings set ${Date.now()}`;
    await testPage.getByTestId("repository-set-create").click();
    await testPage.getByTestId(EDITOR_NAME).fill(setName);
    await testPage.getByTestId("repository-set-add-repository").click();
    await testPage.getByRole("option", { name: /E2E Repo/ }).click();
    await testPage.getByTestId("repository-set-add-repository").click();
    await testPage.getByRole("option", { name: SECOND_REPO_NAME }).click();
    const basePicker = testPage.getByTestId(`repository-set-base-${second.id}`);
    await basePicker.click();
    const dropdown = testPage.getByTestId(`repository-set-base-dropdown-${second.id}`);
    await expect(dropdown).toBeVisible();
    await expect(dropdown.getByPlaceholder("Search branches...")).toBeVisible();
    await expect(dropdown.getByText("Branches")).toBeVisible();
    await expect(dropdown.getByRole("option", { name: /^main local/ })).toBeVisible();
    await expect(dropdown.getByRole("option", { name: /^origin\/main origin/ })).toBeVisible();
    await expect(dropdown.getByText("origin", { exact: true })).toBeVisible();

    const search = dropdown.getByPlaceholder("Search branches...");
    await search.fill("origin");
    await expect(dropdown.getByRole("option", { name: /^origin\/main origin/ })).toBeVisible();
    await expect(dropdown.getByRole("option", { name: /^main local/ })).toHaveCount(0);
    await dropdown.getByTestId("branch-refresh-button").click();
    await expect(dropdown.getByRole("option", { name: /^origin\/main origin/ })).toBeVisible();

    await search.fill("");
    const develop = dropdown.getByRole("option", { name: /^develop local/ });
    await expect(develop).toBeVisible();
    await develop.click();
    await prCapture.screenshot("desktop-repository-set-editor", {
      caption: "The desktop repository set editor assigns a saved base branch per member.",
    });
    await testPage.getByTestId(EDITOR_SAVE).click();

    const rows = testPage.getByTestId(SET_ROW);
    await expect(rows).toHaveCount(1);
    await expect(rows.first()).toHaveAttribute("data-member-count", "2");

    await expect
      .poll(async () => {
        const listed = await apiClient.listRepositorySets(seedData.workspaceId);
        const stored = listed.repository_sets.find((entry) => entry.name === setName);
        return (
          stored?.repositories.length === 2 &&
          stored.repositories.some((member) => member.repository_id === seedData.repositoryId) &&
          stored.repositories.some((member) => member.repository_id === second.id) &&
          stored.repositories.find((member) => member.repository_id === second.id)?.base_branch ===
            "develop"
        );
      })
      .toBe(true);

    const createdId = (
      await apiClient.listRepositorySets(seedData.workspaceId)
    ).repository_sets.find((entry) => entry.name === setName)!.id;

    // Edit: drop one member. A supplied membership replaces the whole list.
    await testPage.getByTestId(`repository-set-edit-${createdId}`).click();
    await testPage.getByTestId(`repository-set-remove-${second.id}`).click();
    await testPage.getByTestId(EDITOR_SAVE).click();

    await expect(rows.first()).toHaveAttribute("data-member-count", "1");
    await expect
      .poll(async () => {
        const listed = await apiClient.listRepositorySets(seedData.workspaceId);
        return listed.repository_sets.find((entry) => entry.id === createdId)?.repositories.length;
      })
      .toBe(1);

    // Delete: the set goes, the repositories stay.
    await testPage.getByTestId(`repository-set-delete-${createdId}`).click();
    const confirmation = testPage.getByTestId("repository-set-delete-confirm-popover");
    await expect(confirmation).toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await testPage.getByTestId("repository-set-delete-confirm").click();

    await expect(testPage.getByTestId("repository-sets-empty")).toBeVisible();
    const repositories = await apiClient.listRepositories(seedData.workspaceId);
    expect(repositories.repositories.map((entry) => entry.id)).toEqual(
      expect.arrayContaining([seedData.repositoryId, second.id]),
    );
  });

  test("a set survives losing a member repository and lists as smaller", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const dir = path.join(backend.tmpDir, "repos", "settings-sets-deleted-member");
    fs.mkdirSync(dir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: dir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: dir, env: gitEnv });
    const doomed = await apiClient.createRepository(seedData.workspaceId, dir, "main", {
      name: "Doomed Repository",
    });
    const setName = `Losing a member ${Date.now()}`;
    await apiClient.createRepositorySet(seedData.workspaceId, setName, [
      seedData.repositoryId,
      doomed.id,
    ]);

    await apiClient.rawRequest("DELETE", `/api/v1/repositories/${doomed.id}`);

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    const rows = testPage.getByTestId(SET_ROW);
    await expect(rows).toHaveCount(1);
    // The set is kept, with the member that still exists.
    await expect(rows.first()).toContainText(setName);
    await expect(rows.first()).toHaveAttribute("data-member-count", "1");
  });
});
