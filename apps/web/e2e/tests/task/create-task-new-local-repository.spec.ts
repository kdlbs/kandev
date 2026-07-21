import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

type PersistedRepository = {
  id: string;
  name: string;
  local_path: string;
  default_branch: string;
  source_type: string;
};

async function listRepositories(
  apiClient: ApiClient,
  workspaceId: string,
): Promise<PersistedRepository[]> {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${workspaceId}/repositories`,
  );
  expect(response.ok).toBe(true);
  return ((await response.json()) as { repositories: PersistedRepository[] }).repositories;
}

async function openCreateTask(page: Page): Promise<void> {
  const kanban = new KanbanPage(page);
  await kanban.goto();
  await kanban.createTaskButton.first().click();
  await expect(page.getByTestId("create-task-dialog")).toBeVisible();
}

async function openRepositoryCreation(page: Page): Promise<void> {
  await page.getByTestId("repo-chip-trigger").first().click();
  const action = page.getByRole("option", { name: "Create new repository" });
  await expect(action).toBeVisible();
  await action.click();
  await expect(page.getByTestId("create-local-repository-dialog")).toBeVisible();
}

async function createRepository(page: Page, name: string, targetPath: string): Promise<void> {
  await page.getByRole("textbox", { name: "Repository name" }).fill(name);
  await expect(page.getByTitle(targetPath)).toBeVisible();
  await page.getByRole("button", { name: "Create repository" }).click();
  await expect(page.getByTestId("create-local-repository-dialog")).not.toBeVisible();
}

function expectUnbornMainRepository(repositoryPath: string): void {
  expect(fs.statSync(path.join(repositoryPath, ".git")).isDirectory()).toBe(true);
  expect(
    execFileSync("git", ["symbolic-ref", "--short", "HEAD"], {
      cwd: repositoryPath,
      encoding: "utf8",
    }).trim(),
  ).toBe("main");
  expect(
    spawnSync("git", ["rev-parse", "--verify", "HEAD"], { cwd: repositoryPath }).status,
  ).not.toBe(0);
}

function taskIdFromUrl(page: Page): string {
  const match = page.url().match(/\/t\/([^/?]+)/);
  if (!match) throw new Error(`Task route missing from ${page.url()}`);
  return match[1];
}

test.describe("Create task with a new local repository", () => {
  test("initializes, registers, selects, and starts from an unborn main repository", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repositoryName = "desktop-unborn-main";
    const repositoryPath = path.join(backend.tmpDir, repositoryName);
    const { executors } = await apiClient.listExecutors();
    const directExecutor = executors.find((executor) =>
      ["local", "local_pc"].includes(executor.type),
    );
    const directProfile = directExecutor?.profiles?.[0];
    expect(directExecutor, "a direct local executor is required by the fixture").toBeDefined();
    expect(
      directProfile,
      "a direct local executor profile is required by the fixture",
    ).toBeDefined();

    await openCreateTask(testPage);
    await testPage.getByTestId("task-title-input").fill("Task on a new local repository");
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");
    await testPage.getByTestId("executor-profile-selector").click();
    await testPage.getByRole("option", { name: /Worktree/i }).click();

    await openRepositoryCreation(testPage);
    await expect(testPage.getByTestId("create-local-repository-dialog")).toContainText(
      /will switch to/i,
    );
    await createRepository(testPage, repositoryName, repositoryPath);

    await expect(testPage.getByTestId("repo-chip-trigger").first()).toContainText(repositoryName);
    await expect(testPage.getByTestId("branch-chip-trigger").first()).toContainText("main");
    await expect(testPage.getByTestId("executor-profile-selector")).toContainText(
      directExecutor!.name,
    );
    await expect(testPage.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 30_000 });
    await testPage.getByTestId("submit-start-agent").click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const persisted = (await listRepositories(apiClient, seedData.workspaceId)).find(
      (repository) => repository.local_path === repositoryPath,
    );
    expect(persisted).toMatchObject({
      name: repositoryName,
      local_path: repositoryPath,
      default_branch: "main",
      source_type: "local",
    });
    const taskId = taskIdFromUrl(testPage);
    const task = await apiClient.getTask(taskId);
    expect(task.repositories).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ repository_id: persisted!.id, base_branch: "main" }),
      ]),
    );
    await expect
      .poll(async () => await apiClient.getTaskEnvironment(taskId), { timeout: 20_000 })
      .toMatchObject({
        executor_type: expect.stringMatching(/^(local|local_pc)$/),
        executor_profile_id: directProfile!.id,
      });
    expectUnbornMainRepository(repositoryPath);
  });

  test("leaves a conflicting target untouched and allows retry with a new name", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const conflictName = "desktop-existing-target";
    const retryName = "desktop-conflict-retry";
    const conflictPath = path.join(backend.tmpDir, conflictName);
    const retryPath = path.join(backend.tmpDir, retryName);
    const sentinelPath = path.join(conflictPath, "keep.txt");
    fs.mkdirSync(conflictPath);
    fs.writeFileSync(sentinelPath, "do not modify\n");
    const repositoriesBefore = await listRepositories(apiClient, seedData.workspaceId);

    await openCreateTask(testPage);
    await openRepositoryCreation(testPage);
    const nameInput = testPage.getByRole("textbox", { name: "Repository name" });
    await nameInput.fill(conflictName);
    await testPage.getByRole("button", { name: "Create repository" }).click();

    await expect(testPage.getByTestId("create-local-repository-dialog")).toBeVisible();
    await expect(testPage.getByRole("alert")).toContainText(/exist|conflict/i);
    await expect(nameInput).toHaveValue(conflictName);
    expect(fs.readFileSync(sentinelPath, "utf8")).toBe("do not modify\n");
    expect(fs.existsSync(path.join(conflictPath, ".git"))).toBe(false);
    expect(await listRepositories(apiClient, seedData.workspaceId)).toHaveLength(
      repositoriesBefore.length,
    );

    await nameInput.fill(retryName);
    await createRepository(testPage, retryName, retryPath);
    await expect(testPage.getByTestId("repo-chip-trigger").first()).toContainText(retryName);
    expectUnbornMainRepository(retryPath);
  });
});
