import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";

const FIXTURE_PROVIDER = "fixture-source-control";
const FIXTURE_REPOSITORY_ID = "fixture-repository";
const FIXTURE_REPOSITORY_URL = "https://bitbucket.example.test/projects/TEAM/repos/fixture";
const FIXTURE_BRANCH = "feature/provider-contract";

type RepositorySummary = {
  id: string;
  provider?: string;
  provider_repo_id?: string;
};

async function fixtureRepositories(apiClient: ApiClient, workspaceId: string) {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${workspaceId}/repositories`,
  );
  const bodyText = await response.text();
  expect(response.ok, bodyText).toBe(true);
  const body = JSON.parse(bodyText) as { repositories?: RepositorySummary[] };
  return (body.repositories ?? []).filter(
    (repository) =>
      repository.provider === FIXTURE_PROVIDER &&
      repository.provider_repo_id === FIXTURE_REPOSITORY_ID,
  );
}

async function removeFixtureRepositories(apiClient: ApiClient, workspaceId: string): Promise<void> {
  for (const repository of await fixtureRepositories(apiClient, workspaceId)) {
    const response = await apiClient.rawRequest("DELETE", `/api/v1/repositories/${repository.id}`);
    if (!response.ok && response.status !== 404) {
      throw new Error(
        `fixture repository cleanup failed (${response.status}): ${await response.text()}`,
      );
    }
  }
}

async function selectFixtureRepository(page: Page): Promise<void> {
  await page.getByTestId("source-mode-remote").click();
  const repositoryTrigger = page.getByTestId("remote-repo-chip-trigger");
  await expect(repositoryTrigger).toHaveCount(1);
  await repositoryTrigger.click();
  const repositoryOption = page
    .getByTestId("remote-repo-option")
    .filter({ hasText: "TEAM/fixture" });
  await expect(repositoryOption).toHaveCount(1);
  await expect(repositoryOption).toBeVisible({ timeout: 15_000 });
  await repositoryOption.click();
  await expect(repositoryTrigger).toContainText("TEAM/fixture");
}

async function selectFixtureBranch(page: Page): Promise<void> {
  const branch = page.getByTestId("remote-branch-chip-trigger");
  await expect(branch).toHaveCount(1);
  await expect(branch).toBeEnabled({ timeout: 15_000 });
  await branch.click();
  const option = page.getByRole("option", { name: FIXTURE_BRANCH, exact: false });
  await expect(option).toHaveCount(1);
  await expect(option).toBeVisible({ timeout: 15_000 });
  await option.click();
  await expect(branch).toContainText(FIXTURE_BRANCH);
}

async function fillTaskForm(page: Page, title: string): Promise<void> {
  await page.getByTestId("task-title-input").fill(title);
  await page
    .getByTestId("task-description-input")
    .fill("Create this task from a plugin repository.");
}

async function createWithoutAgent(page: Page): Promise<Response> {
  const responsePromise = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
  );
  await page.getByTestId("submit-start-agent-chevron").click();
  await page.getByTestId("submit-create-without-agent").click();
  return responsePromise;
}

test.describe("first-use plugin repository task creation", () => {
  test.afterEach(async ({ apiClient, seedData }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
    await removeFixtureRepositories(apiClient, seedData.workspaceId).catch(() => undefined);
  });

  test("creates and persists the authoritative repository on desktop", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await removeFixtureRepositories(apiClient, seedData.workspaceId);
    expect(await fixtureRepositories(apiClient, seedData.workspaceId)).toHaveLength(0);
    await installFixturePlugin(testPage);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await selectFixtureRepository(testPage);
    await selectFixtureBranch(testPage);
    await fillTaskForm(testPage, "First-use plugin repository task");

    const response = await createWithoutAgent(testPage);
    const responseBody = await response.text();
    expect(response.status(), responseBody).toBe(200);
    const created = JSON.parse(responseBody) as { id: string };
    expect(created.id).toBeTruthy();
    await expect(dialog).toBeHidden({ timeout: 15_000 });

    const task = await apiClient.getTask(created.id);
    expect(task.repositories).toHaveLength(1);
    expect(task.repositories?.[0]).toMatchObject({
      base_branch: "main",
      checkout_branch: FIXTURE_BRANCH,
    });
    const repositories = await fixtureRepositories(apiClient, seedData.workspaceId);
    expect(repositories).toHaveLength(1);
    const repositoryResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/repositories/${repositories[0]!.id}`,
    );
    const repositoryBody = await repositoryResponse.text();
    expect(repositoryResponse.ok, repositoryBody).toBe(true);
    expect(JSON.parse(repositoryBody)).toMatchObject({
      provider: FIXTURE_PROVIDER,
      provider_repo_id: FIXTURE_REPOSITORY_ID,
      provider_owner: "TEAM",
      provider_name: "fixture",
      remote_url: "https://bitbucket.example.test/scm/TEAM/fixture.git",
      default_branch: "main",
    });
  });

  test("keeps the form open when the provider becomes inactive", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await removeFixtureRepositories(apiClient, seedData.workspaceId);
    expect(await fixtureRepositories(apiClient, seedData.workspaceId)).toHaveLength(0);
    await installFixturePlugin(testPage);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await selectFixtureRepository(testPage);
    await selectFixtureBranch(testPage);
    const title = "Inactive plugin repository task";
    await fillTaskForm(testPage, title);

    const disable = await apiClient.rawRequest("POST", `/api/plugins/${PLUGIN_ID}/disable`);
    expect(disable.ok, await disable.text()).toBe(true);
    const response = await createWithoutAgent(testPage);
    expect(response.status()).toBe(503);
    await expect(dialog).toBeVisible();
    await expect(
      testPage.getByText(
        "The repository provider is unavailable. Check the connection and try again.",
        {
          exact: true,
        },
      ),
    ).toBeVisible();
    const tasks = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.tasks.some((task) => task.title === title)).toBe(false);
    expect(await fixtureRepositories(apiClient, seedData.workspaceId)).toHaveLength(0);
    expect(testPage.url()).not.toContain(FIXTURE_REPOSITORY_URL);
  });
});
