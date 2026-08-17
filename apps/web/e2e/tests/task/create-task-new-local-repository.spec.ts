import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";

useRegularMode();

// The two backend calls this flow actually waits on. `initialize-local` creates
// the repository on disk and registers it; the workspace-scoped branches read is
// what fills the branch chip, and until it lands `hasAllBranches` is false and
// every submit button in the footer stays disabled. Asserting on the buttons
// instead of on these two responses is what made this spec flaky: it waited
// 30s for `submit-start-agent` to enable and never learned why it did not.
const INITIALIZE_LOCAL_PATH = /\/repositories\/initialize-local$/;
const WORKSPACE_BRANCHES_PATH = /^\/api\/v1\/workspaces\/[^/]+\/branches$/;

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
  await page.getByTestId("repo-chip-trigger").click();
  const search = page.getByPlaceholder("Search repositories...");
  const refresh = page.getByTestId("repo-refresh-button");
  const action = page.getByTestId("create-local-repository-button");
  await expect(search).toBeVisible();
  await expect(refresh).toBeVisible();
  await expect(action).toBeVisible();
  await expect(refresh).toBeEnabled();
  await refresh.hover();
  const refreshTooltip = page
    .locator('[data-slot="tooltip-content"][data-state]')
    .getByText("Refresh repositories", { exact: true })
    .filter({ visible: true });
  await expect(refreshTooltip).toBeVisible();
  const [searchBox, refreshBox, actionBox] = await Promise.all([
    search.boundingBox(),
    refresh.boundingBox(),
    action.boundingBox(),
  ]);
  expect(searchBox).not.toBeNull();
  expect(refreshBox).not.toBeNull();
  expect(actionBox).not.toBeNull();
  expect(searchBox!.width).toBeGreaterThanOrEqual(280);
  expect(searchBox!.x + searchBox!.width).toBeLessThanOrEqual(refreshBox!.x);
  expect(refreshBox!.x + refreshBox!.width).toBeLessThanOrEqual(actionBox!.x);
  await action.click();
  await expect(page.getByTestId("create-local-repository-dialog")).toBeVisible();
}

async function createRepository(page: Page, name: string, targetPath: string): Promise<void> {
  await page.getByRole("textbox", { name: "Parent directory" }).fill(path.dirname(targetPath));
  await page.getByRole("textbox", { name: "Repository name" }).fill(name);
  await expect(page.getByTitle(targetPath)).toBeVisible();
  // Armed before the click: a 201 here is the cause, the dialog closing is the
  // effect. A rejected create now fails naming the response it never saw,
  // rather than as an anonymous "dialog stayed open" timeout.
  const created = waitForHttp(page, "POST", INITIALIZE_LOCAL_PATH, {
    predicate: (response) => response.status() === 201,
  });
  await page.getByRole("button", { name: "Create repository" }).click();
  await created;
  await expect(page.getByTestId("create-local-repository-dialog")).not.toBeVisible();
}

function expectMainRepository(repositoryPath: string): void {
  expect(fs.statSync(path.join(repositoryPath, ".git")).isDirectory()).toBe(true);
  expect(
    execFileSync("git", ["symbolic-ref", "--short", "HEAD"], {
      cwd: repositoryPath,
      encoding: "utf8",
    }).trim(),
  ).toBe("main");
  const branchRef = spawnSync("git", ["show-ref", "--verify", "--quiet", "refs/heads/main"], {
    cwd: repositoryPath,
  });
  expect(branchRef.error).toBeUndefined();
  expect(branchRef.status).toBe(0);

  const head = spawnSync("git", ["rev-parse", "--verify", "HEAD"], { cwd: repositoryPath });
  expect(head.error).toBeUndefined();
  expect(head.status).toBe(0);

  const commitCount = spawnSync("git", ["rev-list", "--count", "HEAD"], {
    cwd: repositoryPath,
    encoding: "utf8",
  });
  expect(commitCount.error).toBeUndefined();
  expect(commitCount.status).toBe(0);
  expect(String(commitCount.stdout).trim()).toBe("1");

  const tree = spawnSync("git", ["ls-tree", "-r", "--name-only", "HEAD"], {
    cwd: repositoryPath,
    encoding: "utf8",
  });
  expect(tree.error).toBeUndefined();
  expect(tree.status).toBe(0);
  expect(String(tree.stdout).trim()).toBe("");
}

function taskIdFromUrl(page: Page): string {
  const match = page.url().match(/\/t\/([^/?]+)/);
  if (!match) throw new Error(`Task route missing from ${page.url()}`);
  return match[1];
}

test.describe("Create task with a new local repository", () => {
  test("initializes, registers, selects, and starts from a real main repository", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repositoryName = "desktop-real-main";
    const repositoryPath = path.join(
      backend.tmpDir,
      "missing-manual-parent",
      "nested",
      repositoryName,
    );
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
    // Armed before the create: selecting the freshly registered repository
    // triggers this read, and the branch chip cannot enable until it returns.
    const branchesLoaded = waitForHttp(testPage, "GET", WORKSPACE_BRANCHES_PATH);
    await createRepository(testPage, repositoryName, repositoryPath);
    expect(fs.statSync(path.dirname(repositoryPath)).isDirectory()).toBe(true);

    await expect(testPage.getByTestId("repo-chip-trigger")).toContainText(repositoryName);
    await branchesLoaded;
    // No budget from here on: the cause has landed, so anything left is a render.
    const branchSelector = testPage.getByTestId("branch-chip-trigger").first();
    await expect(branchSelector).toBeEnabled();
    await branchSelector.click();
    const mainOption = testPage.getByRole("option", { name: /^main\b/ }).first();
    await expect(mainOption).toBeVisible();
    await mainOption.click();
    await expect(branchSelector).toContainText("main");
    await expect(testPage.getByTestId("executor-profile-selector")).toContainText(
      directExecutor!.name,
    );
    await expect(testPage.getByTestId("submit-start-agent")).toBeEnabled();
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
    expectMainRepository(repositoryPath);
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
    await expect(testPage.getByTestId("repo-chip-trigger")).toContainText(retryName);
    expectMainRepository(retryPath);
  });
});
