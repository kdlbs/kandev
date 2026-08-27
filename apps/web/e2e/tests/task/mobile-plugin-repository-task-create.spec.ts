import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import type { ApiClient } from "../../helpers/api-client";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

const FIXTURE_PROVIDER = "fixture-source-control";
const FIXTURE_REPOSITORY_ID = "fixture-repository";
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
    await apiClient
      .rawRequest("DELETE", `/api/v1/repositories/${repository.id}`)
      .catch(() => undefined);
  }
}

async function selectFixtureRepository(page: Page): Promise<void> {
  await page.getByTestId("source-mode-remote").tap();
  const repositoryTrigger = page.getByTestId("remote-repo-chip-trigger");
  await expect(repositoryTrigger).toHaveCount(1);
  await repositoryTrigger.tap();
  const repositoryOption = page
    .getByTestId("remote-repo-option")
    .filter({ hasText: "TEAM/fixture" });
  await expect(repositoryOption).toHaveCount(1);
  await expect(repositoryOption).toBeVisible({ timeout: 15_000 });
  await repositoryOption.tap();
  await expect(repositoryTrigger).toContainText("TEAM/fixture");
}

async function selectFixtureBranch(page: Page): Promise<void> {
  const branch = page.getByTestId("remote-branch-chip-trigger");
  await expect(branch).toHaveCount(1);
  await expect(branch).toBeEnabled({ timeout: 15_000 });
  await branch.tap();
  const option = page.getByRole("option", { name: FIXTURE_BRANCH, exact: false });
  await expect(option).toHaveCount(1);
  await expect(option).toBeVisible({ timeout: 15_000 });
  await option.tap();
  await expect(branch).toContainText(FIXTURE_BRANCH);
}

test.describe("first-use plugin repository task creation on mobile", () => {
  test.afterEach(async ({ apiClient, seedData }) => {
    await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
    await removeFixtureRepositories(apiClient, seedData.workspaceId);
  });

  test("creates the same authoritative repository through the phone dialog", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await removeFixtureRepositories(apiClient, seedData.workspaceId);
    expect(await fixtureRepositories(apiClient, seedData.workspaceId)).toHaveLength(0);
    await installFixturePlugin(testPage);

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.tap();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await selectFixtureRepository(testPage);
    await selectFixtureBranch(testPage);
    await dialog.getByTestId("task-title-input").fill("Mobile first-use plugin repository task");
    await dialog
      .getByTestId("task-description-input")
      .fill("Create this task from a plugin repository on a phone.");

    const createOnly = dialog.getByRole("button", { name: "Create only", exact: true });
    await expect(createOnly).toBeEnabled();
    const createBox = await createOnly.boundingBox();
    expect(createBox).not.toBeNull();
    expect(createBox!.height).toBeGreaterThanOrEqual(44);
    expect(createBox!.width).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(
      testPage,
      "plugin repository task dialog before submit",
    );

    const responsePromise = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
    );
    await createOnly.tap();
    const response = await responsePromise;
    const responseBody = await response.text();
    expect(response.status(), responseBody).toBe(200);
    const created = JSON.parse(responseBody) as { id: string };
    expect(created.id).toBeTruthy();
    await expect(dialog).toBeHidden({ timeout: 15_000 });
    await assertNoDocumentHorizontalOverflow(
      testPage,
      "plugin repository task dialog after submit",
    );

    const task = await apiClient.getTask(created.id);
    expect(task.repositories).toHaveLength(1);
    expect(task.repositories?.[0]).toMatchObject({
      base_branch: "main",
      checkout_branch: FIXTURE_BRANCH,
    });
    expect(await fixtureRepositories(apiClient, seedData.workspaceId)).toHaveLength(1);
  });
});
