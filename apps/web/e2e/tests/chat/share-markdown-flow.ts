import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

export async function verifyShareMarkdownPreview(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<void> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Share rendered markdown",
    seedData.agentProfileId,
    {
      description: "/e2e:markdown-table",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  await testPage.getByTestId("share-task-button").click();
  const dialog = testPage.getByRole("dialog", { name: "Share this task" });
  await expect(dialog.getByRole("heading", { level: 2, name: "Summary" })).toBeVisible();
  await expect(dialog.locator("table")).toBeVisible();

  expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth + 1)).toBe(
    true,
  );
  await expect(dialog.getByRole("button", { name: "Publish to GitHub Gist" })).toBeVisible();
}
