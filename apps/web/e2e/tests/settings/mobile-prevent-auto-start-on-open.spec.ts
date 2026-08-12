import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

// Mobile variant of the final-step gate: the gating hooks run on mobile
// through the shared responsive TaskPageContent, so the Start agent button
// must appear on a phone viewport too. Mirrors mobile-general-settings.spec.ts.

test.describe("Mobile prevent auto-start on open", () => {
  test("final-step task with no session opens with the Start agent button when the preference is on", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 390, height: 844 });
    const finalStep = await apiClient.createWorkflowStep(
      seedData.workflowId,
      "Prevent AutoStart Mobile Done",
      7,
      { events: { on_enter: [{ type: "auto_start_agent" }] } },
    );
    let taskId: string | undefined;
    try {
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
      const task = await apiClient.createTask(
        seedData.workspaceId,
        "Prevent AutoStart Mobile Task",
        {
          description: "Prevent auto-start on open final step task",
          workflow_id: seedData.workflowId,
          workflow_step_id: finalStep.id,
          agent_profile_id: seedData.agentProfileId,
          repository_ids: [seedData.repositoryId],
        },
      );
      taskId = task.id;

      await testPage.goto(`/t/${task.id}`);

      const startButton = testPage.getByTestId("task-description-start-button");
      await expect(startButton).toBeVisible({ timeout: 30_000 });
      await expect(testPage.getByText("Started agent", { exact: false })).toHaveCount(0);

      await startButton.click();
      const session = new SessionPage(testPage);
      await expect(testPage.getByText("Started agent", { exact: false })).toBeVisible({
        timeout: 30_000,
      });
      await session.sendMessageViaButton("/e2e:simple-message");
      await session.expectChatResponseVisible("simple mock response", 0, { timeout: 30_000 });
    } finally {
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: false });
      if (taskId) await apiClient.deleteTask(taskId);
      await apiClient.deleteWorkflowStep(finalStep.id);
    }
  });
});
