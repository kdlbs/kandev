import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { makeGitEnv } from "../../helpers/git-helper";
import { openTaskRepositoryPicker } from "../../helpers/task-repository-picker";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

type TaskRepositoryRow = {
  repository_id: string;
  base_branch?: string;
  position: number;
};

type RepositoryRecord = {
  id: string;
  provider?: string;
  provider_owner?: string;
  provider_name?: string;
};

async function seedRemoteRepository(apiClient: ApiClient): Promise<void> {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubAddRepos("mock-user", [
    { full_name: "mock-user/mixed-alpha", owner: "mock-user", name: "mixed-alpha", private: false },
  ]);
  await apiClient.mockGitHubAddBranches("mock-user", "mixed-alpha", [
    { name: "main" },
    { name: "develop" },
  ]);
}

async function createLocalTarget(
  apiClient: ApiClient,
  backendTmpDir: string,
  workspaceId: string,
): Promise<{ id: string }> {
  const repositoryPath = path.join(backendTmpDir, "repos", "mixed-local-target");
  fs.mkdirSync(repositoryPath, { recursive: true });
  const gitEnv = makeGitEnv(backendTmpDir);
  execSync("git init -b main", { cwd: repositoryPath, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: repositoryPath, env: gitEnv });
  execSync("git checkout -b develop", { cwd: repositoryPath, env: gitEnv });
  return apiClient.createRepository(workspaceId, repositoryPath, "main", {
    name: "Mixed Local Target",
  });
}

async function getTaskRepositories(
  apiClient: ApiClient,
  taskId: string,
): Promise<TaskRepositoryRow[]> {
  const response = await apiClient.rawRequest("GET", `/api/v1/tasks/${taskId}`);
  expect(response.ok).toBe(true);
  const body = (await response.json()) as { repositories?: TaskRepositoryRow[] };
  return body.repositories ?? [];
}

async function getRepository(
  apiClient: ApiClient,
  repositoryId: string,
): Promise<RepositoryRecord> {
  const response = await apiClient.rawRequest("GET", `/api/v1/repositories/${repositoryId}`);
  expect(response.ok).toBe(true);
  return (await response.json()) as RepositoryRecord;
}

test.describe("Create task mixed repository selection", () => {
  test.describe.configure({ retries: 1 });

  test("submits local, remote, and local rows in insertion order", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    await seedRemoteRepository(apiClient);
    const secondLocal = await createLocalTarget(apiClient, backend.tmpDir, seedData.workspaceId);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByTestId("repo-chip").first()).toContainText("E2E Repo");

    await openTaskRepositoryPicker(testPage, { provider: "github" });
    await testPage
      .getByTestId("task-repository-remote-option")
      .filter({ hasText: "mock-user/mixed-alpha" })
      .click();

    await openTaskRepositoryPicker(testPage, { provider: "local" });
    await testPage
      .getByTestId("task-repository-local-option")
      .filter({ hasText: "Mixed Local Target" })
      .click();

    await expect(dialog.getByTestId("repo-chip")).toHaveCount(2);
    await expect(dialog.getByTestId("remote-repo-chip")).toHaveCount(1);
    await expect(dialog.getByTestId("repo-chip").nth(0)).toContainText("E2E Repo");
    await expect(dialog.getByTestId("remote-repo-chip")).toContainText("mixed-alpha");
    await expect(dialog.getByTestId("repo-chip").nth(1)).toContainText("Mixed Local Target");
    await waitForFiniteAnimations(dialog);
    await prCapture.screenshot("desktop-mixed-repository-task", {
      caption: "A desktop task draft can show local and remote repositories in one ordered list.",
    });

    await dialog.getByTestId("task-title-input").fill("Mixed repository task");
    await dialog.getByTestId("task-description-input").fill("Create a task from mixed sources");
    const submit = dialog.getByTestId("submit-start-agent");
    await expect(submit).toBeEnabled({ timeout: 15_000 });
    const responsePromise = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
    );
    await submit.click();
    const response = await responsePromise;
    expect(response.ok()).toBe(true);
    const created = (await response.json()) as { id: string };
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    const rows = [...(await getTaskRepositories(apiClient, created.id))].sort(
      (first, second) => first.position - second.position,
    );
    expect(rows).toHaveLength(3);
    expect(rows[0].repository_id).toBe(seedData.repositoryId);
    expect(rows[2].repository_id).toBe(secondLocal.id);
    expect(rows[2].base_branch).toBe("main");
    const remote = await getRepository(apiClient, rows[1].repository_id);
    expect(remote).toMatchObject({
      provider: "github",
      provider_owner: "mock-user",
      provider_name: "mixed-alpha",
    });
  });

  test("removing the last row leaves an explicit empty draft", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const remoteRemove = dialog.getByTestId("remote-chip-remove");
    while ((await remoteRemove.count()) > 0) {
      await remoteRemove.first().click();
    }
    const localRemove = dialog.getByTestId("remove-repo-chip");
    while ((await localRemove.count()) > 0) {
      await localRemove.first().click();
    }

    await expect(dialog.getByTestId("repo-chip")).toHaveCount(0);
    await expect(dialog.getByTestId("remote-repo-chip")).toHaveCount(0);
    const add = dialog.getByTestId("add-repository");
    await expect(add).toBeVisible();
    await expect(add).toContainText("Add Repository/Folder");
    await add.click();
    const sourceOptions = testPage.getByTestId("workspace-source-menu-options");
    await expect(sourceOptions).toBeVisible();
    // Worktree can host a folder directly, including after the last repository
    // is removed and the draft returns to its explicit empty state.
    await expect(sourceOptions.getByTestId("workspace-source-menu-folder")).toBeEnabled();
    await testPage.keyboard.press("Escape");
  });
});
