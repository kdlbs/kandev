import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

async function seedAndOpenTask(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Model Selector",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

test.describe("Mobile chat model selector", () => {
  test.describe.configure({ retries: 1, timeout: 60_000 });

  test("shows mode and model in the swipeable composer actions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedAndOpenTask(testPage, apiClient, seedData);

    const leftActions = testPage.getByTestId("mobile-chat-toolbar-left-actions");
    await expect(leftActions).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("toolbar-overflow-menu")).not.toBeVisible();
    await expect(leftActions.getByTestId("toolbar-item-sessions")).toHaveCount(0);

    await expect(leftActions.getByTestId("session-mode-selector")).toBeVisible();

    const trigger = leftActions.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toContainText("Mock Fast", { timeout: 15_000 });

    await trigger.tap();
    await expect(testPage.getByRole("option", { name: /Mock Smart/ })).toBeVisible({
      timeout: 5_000,
    });
  });
});
