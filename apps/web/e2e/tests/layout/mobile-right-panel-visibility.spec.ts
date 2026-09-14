import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { openTaskSession } from "../../helpers/session";

async function createTask(apiClient: ApiClient, seedData: SeedData) {
  return apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile right-panel navigation",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
}

test.describe("mobile right-panel navigation", () => {
  test("keeps Files and Terminal in the existing full-screen navigation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await createTask(apiClient, seedData);
    const session = await openTaskSession(testPage, task.id);

    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
    await expect(testPage.getByTestId("tablet-task-layout")).toHaveCount(0);
    await expect(testPage.getByTestId("task-right-panels-toggle")).toHaveCount(0);

    const chat = testPage.getByRole("button", { name: "Chat", exact: true });
    const files = testPage.getByRole("button", { name: "Files", exact: true });
    const terminal = testPage.getByRole("button", { name: "Terminal", exact: true });
    await expect(chat).toBeVisible();
    await expect(files).toBeVisible();
    await expect(terminal).toBeVisible();

    await files.tap();
    await expect(testPage.getByTestId("file-tree-scroll")).toBeVisible();
    await terminal.tap();
    await expect(session.terminal).toBeVisible();
    await chat.tap();
    await expect(session.activeChat()).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "mobile right-panel navigation");
  });
});
