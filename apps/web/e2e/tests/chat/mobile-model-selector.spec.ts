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
  return { session, task };
}

test.describe("Mobile chat model selector", () => {
  test.describe.configure({ retries: 1, timeout: 60_000 });

  test("shows compact changes and provider descriptions by touch", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const { task } = await seedAndOpenTask(testPage, apiClient, seedData);

    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const baseline = sessions[0]?.metadata?.acp_config_baseline as
          | Record<string, string>
          | undefined;
        return baseline?.effort;
      })
      .toBe("medium");

    const leftActions = testPage.getByTestId("mobile-chat-toolbar-left-actions");
    await expect(leftActions).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("toolbar-overflow-menu")).not.toBeVisible();
    await expect(leftActions.getByTestId("toolbar-item-sessions")).toHaveCount(0);

    await expect(leftActions.getByTestId("session-mode-selector")).toBeVisible();

    const trigger = leftActions.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toHaveText("Mock Fast", { timeout: 15_000 });

    await trigger.tap();
    await expect(testPage.getByRole("option", { name: /Mock Smart/ })).toBeVisible({
      timeout: 5_000,
    });
    await expect(
      testPage.getByRole("option", { name: /Mock Fast/ }).getByTitle("Fast mock model for testing"),
    ).toBeVisible();
    await expect(
      testPage
        .getByRole("option", { name: /Mock Smart/ })
        .getByTitle("Smart mock model for testing"),
    ).toBeVisible();

    const effortTrigger = testPage.getByTestId("config-option-trigger-effort");
    await expect(effortTrigger).toBeVisible();
    await effortTrigger.tap();
    await expect(testPage.getByTestId("config-option-section-effort")).toBeVisible();
    await testPage.getByRole("button", { name: "Low", exact: true }).tap();
    await expect(trigger).toHaveText("Mock Fast / Low");
    await expect(trigger).not.toContainText("Medium");

    await expect(
      testPage.getByRole("option", { name: /Mock Fast/ }).getByTitle("Fast mock model for testing"),
    ).toBeVisible();
    await testInfo.attach("task-model-selector-mobile", {
      body: await testPage.screenshot(),
      contentType: "image/png",
    });

    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    const popoverBox = await testPage.getByRole("option", { name: /Mock Smart/ }).boundingBox();
    expect(popoverBox).not.toBeNull();
    expect(popoverBox!.x).toBeGreaterThanOrEqual(0);
    expect(popoverBox!.x + popoverBox!.width).toBeLessThanOrEqual(viewport!.width);
  });
});
