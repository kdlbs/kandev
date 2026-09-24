import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { makeGitEnv } from "../../helpers/git-helper";
import { openTaskRepositoryPicker } from "../../helpers/task-repository-picker";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

useRegularMode();

type TaskRepositoryRow = {
  repository_id: string;
  base_branch?: string;
  position: number;
};

type RepositoryRecord = {
  provider?: string;
  provider_owner?: string;
  provider_name?: string;
};

async function seedRemoteRepository(apiClient: ApiClient): Promise<void> {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubAddRepos("mock-user", [
    {
      full_name: "mock-user/mobile-mixed-alpha",
      owner: "mock-user",
      name: "mobile-mixed-alpha",
      private: false,
    },
  ]);
  await apiClient.mockGitHubAddBranches("mock-user", "mobile-mixed-alpha", [
    { name: "main" },
    { name: "develop" },
  ]);
}

async function createLocalTarget(
  apiClient: ApiClient,
  backendTmpDir: string,
  workspaceId: string,
): Promise<{ id: string }> {
  const repositoryPath = path.join(backendTmpDir, "repos", "mobile-mixed-local-target");
  fs.mkdirSync(repositoryPath, { recursive: true });
  const gitEnv = makeGitEnv(backendTmpDir);
  execSync("git init -b main", { cwd: repositoryPath, env: gitEnv });
  execSync('git commit --allow-empty -m "init"', { cwd: repositoryPath, env: gitEnv });
  execSync("git checkout -b develop", { cwd: repositoryPath, env: gitEnv });
  return apiClient.createRepository(workspaceId, repositoryPath, "main", {
    name: "Mobile Mixed Local Target",
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

test.describe("Create task mixed repository selection on mobile", () => {
  test.describe.configure({ retries: 1 });

  test("uses one repository sheet and submits ordered mixed rows", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    await seedRemoteRepository(apiClient);
    const secondLocal = await createLocalTarget(apiClient, backend.tmpDir, seedData.workspaceId);

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.tap();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await openTaskRepositoryPicker(testPage, { mobile: true, provider: "github" });
    const remoteOption = testPage
      .getByTestId("task-repository-remote-option")
      .filter({ hasText: "mock-user/mobile-mixed-alpha" });
    await expect(remoteOption).toBeVisible({ timeout: 15_000 });
    await remoteOption.dispatchEvent("click");

    await openTaskRepositoryPicker(testPage, { mobile: true, provider: "local" });
    const localOption = testPage
      .getByTestId("task-repository-local-option")
      .filter({ hasText: "Mobile Mixed Local Target" });
    await expect(localOption).toBeVisible({ timeout: 10_000 });
    await localOption.dispatchEvent("click");

    await expect(testPage.getByTestId("mobile-repository-sheet-content")).toHaveCount(1);
    await expect(testPage.getByTestId("repo-chip")).toHaveCount(2);
    await expect(testPage.getByTestId("remote-repo-chip")).toHaveCount(1);
    await expect(testPage.getByTestId("repo-chip").nth(1)).toContainText(
      "Mobile Mixed Local Target",
    );
    await testPage.getByTestId("remote-branch-chip-trigger").dispatchEvent("click");
    await expect(testPage.getByRole("heading", { name: "Branch", exact: true })).toBeVisible();
    const mainBranchOption = testPage.getByRole("option").filter({ hasText: "main" }).first();
    await expect(mainBranchOption).toBeVisible();
    await mainBranchOption.dispatchEvent("click");
    await testPage.getByTestId("remote-repo-chip-trigger").dispatchEvent("click");
    await expect(testPage.getByTestId("remote-repo-input")).toBeVisible();
    await testPage
      .getByTestId("remote-repo-option")
      .filter({ hasText: "mock-user/mobile-mixed-alpha" })
      .dispatchEvent("click");
    const repositorySheet = testPage.getByTestId("mobile-repository-sheet-content");
    await waitForFiniteAnimations(repositorySheet);
    await prCapture.screenshot("mobile-mixed-repository-task", {
      caption: "A task draft can show local and remote repositories in one ordered mobile list.",
    });
    await testPage.getByTestId("mobile-repository-done").dispatchEvent("click");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile mixed repository selection");

    await dialog.getByTestId("task-title-input").fill("Mobile mixed repository task");
    await dialog
      .getByTestId("task-description-input")
      .fill("Create a task from mixed mobile sources");
    const submit = dialog.getByTestId("submit-start-agent");
    await expect(submit).toBeEnabled({ timeout: 15_000 });
    const responsePromise = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
    );
    await submit.tap();
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
      provider_name: "mobile-mixed-alpha",
    });
  });
});
