import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("desktop: session history", () => {
  test("hides terminal Dockview helpers until history is explicitly shown", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Desktop session history", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const primary = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
      sessionId: `desktop-history-primary-${task.id}`,
    });
    const helper = await apiClient.seedTaskSession(task.id, {
      state: "COMPLETED",
      agentProfileId: seedData.agentProfileId,
      sessionId: `desktop-history-helper-${task.id}`,
      completedAt: "2026-09-01T00:00:00Z",
    });
    await apiClient.setPrimarySession(primary.session_id);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const layout = testPage.getByTestId("dockview-task-layout");
    const primaryTab = layout.getByTestId(`session-tab-${primary.session_id}`);
    const helperTab = layout.getByTestId(`session-tab-${helper.session_id}`);

    await expect(primaryTab).toBeVisible({ timeout: 15_000 });
    await expect(helperTab).toHaveCount(0);
    await prCapture.screenshot("dockview-session-history-hidden-desktop", {
      caption: "Full-task desktop Dockview with terminal helper sessions hidden by default",
    });

    const historyToggle = layout.getByTestId("dockview-session-history-toggle");
    await expect(historyToggle).toHaveAttribute("aria-label", "Show session history");
    await historyToggle.click();
    await expect(helperTab).toBeVisible({ timeout: 5_000 });
    await expect(primaryTab).toBeVisible();
    await prCapture.screenshot("dockview-session-history-visible-desktop", {
      caption:
        "Full-task desktop Dockview after the explicit history control reveals a terminal helper",
    });

    await expect(historyToggle).toHaveAttribute("aria-label", "Hide session history");
    await historyToggle.click();
    await expect(helperTab).toHaveCount(0);
    await expect(primaryTab).toBeVisible();
  });
});
