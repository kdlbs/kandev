import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

export async function configureSymlinkAndCreateTask(options: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  title: string;
}) {
  const { page, apiClient, seedData, title } = options;
  await page.goto(`/settings/workspace/${seedData.workspaceId}/repositories`);
  await page.getByRole("button", { name: "Edit" }).first().click();

  const input = page.getByTestId(`copy-files-input-${seedData.repositoryId}`);
  await expect(input).toBeVisible();
  await expect(page.getByTestId("copy-files-remote-fallback")).toBeVisible();
  await input.fill(".env:symlink");

  const saved = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/api/v1/repositories/${seedData.repositoryId}`) &&
      response.request().method() === "PATCH" &&
      response.ok(),
  );
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await saved;

  const { executors } = await apiClient.listExecutors();
  const worktreeProfileName = executors
    .flatMap((executor) => executor.profiles ?? [])
    .find((profile) => profile.id === seedData.worktreeExecutorProfileId)?.name;
  if (!worktreeProfileName) throw new Error("worktree executor profile not found");

  const kanban = new KanbanPage(page);
  await kanban.goto();
  const mobileFab = page.getByTestId("mobile-fab");
  if (await mobileFab.isVisible()) {
    await mobileFab.click();
  } else {
    await kanban.createTaskButton.filter({ visible: true }).click();
  }
  await expect(page.getByTestId("create-task-dialog")).toBeVisible();
  await page.getByTestId("task-title-input").fill(title);
  await page.getByTestId("task-description-input").fill("/e2e:simple-message");
  await page.getByTestId("executor-profile-selector").click();
  await page.getByRole("option", { name: worktreeProfileName }).click();
  await expect(page.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 30_000 });
  const createdResponse = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/tasks") &&
      response.request().method() === "POST" &&
      response.ok(),
  );
  await page.getByTestId("submit-start-agent").click();
  const created = (await (await createdResponse).json()) as { id?: string };
  const taskId = created.id;
  if (!taskId) throw new Error("task id missing from create response");
  return taskId;
}
