import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Session tab close guard", () => {
  test("last session tab hides the close button", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(60_000);

    // Create a task with one session
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Single Session Close Guard",
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
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Find the session tab
    const sessionTab = testPage.locator("[data-testid^='session-tab-']").first();
    await expect(sessionTab).toBeVisible({ timeout: 10_000 });

    // The close action inside the session tab should be hidden
    // (DockviewDefaultTab renders .dv-default-tab-action but hideClose hides it via CSS)
    const closeAction = sessionTab.locator(".dv-default-tab-action");
    await expect(closeAction).toBeHidden({ timeout: 5_000 });
  });
});
