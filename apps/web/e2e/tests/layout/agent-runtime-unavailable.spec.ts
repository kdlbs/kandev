import { expect, test } from "../../fixtures/test-base";
import {
  setAgentRuntimeAvailability,
  stubAgentRuntimeRestart,
} from "../../helpers/agent-runtime-availability";
import { KanbanPage } from "../../pages/kanban-page";
import {
  captureAppStatusBarSettings,
  restoreAppStatusBarSettings,
  setAppStatusBarEnabled,
  type AppStatusBarSettingsBaseline,
} from "../../helpers/app-status-bar-settings";

test.describe("Agent runtime availability", () => {
  let baseline: AppStatusBarSettingsBaseline | undefined;

  test.beforeEach(async ({ apiClient }) => {
    baseline = await captureAppStatusBarSettings(apiClient);
    await setAppStatusBarEnabled(apiClient, true);
  });

  test.afterEach(async ({ apiClient }) => {
    await restoreAppStatusBarSettings(apiClient, baseline);
  });

  test("retains the current board, supports restart, and clears after recovery", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const taskTitle = "Runtime availability retained board task";
    const task = await apiClient.createTask(seedData.workspaceId, taskTitle, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const restartRequestCount = await stubAgentRuntimeRestart(testPage);
    const kanban = new KanbanPage(testPage);

    await setAppStatusBarEnabled(apiClient, false);
    await kanban.goto();
    const taskCard = kanban.taskCard(task.id);
    await expect(taskCard).toBeVisible();

    await setAgentRuntimeAvailability(testPage, {
      status: "unavailable",
      reason: "agentctl_exited",
      occurred_at: "2026-08-08T14:22:52Z",
    });

    const alert = testPage.getByTestId("agent-runtime-alert");
    await expect(alert).toBeVisible();
    await expect(alert).toContainText("Local agent runtime stopped");
    await expect(alert).toContainText("saved data remains safe");
    await expect(taskCard).toBeVisible();
    await prCapture.screenshot("agent-runtime-unavailable-desktop", {
      caption: "Persistent desktop agent runtime recovery alert",
    });

    await expect(testPage.getByTestId("app-status-bar")).toHaveCount(0);
    await expect(alert).toBeVisible();

    await alert.getByRole("button", { name: "Restart Kandev" }).click();
    await expect(testPage.getByTestId("restart-progress-dialog")).toHaveAttribute(
      "data-phase",
      "restarting",
    );
    expect(restartRequestCount()).toBe(1);

    await setAgentRuntimeAvailability(testPage, { status: "available" });
    await expect(alert).toHaveCount(0);
    await expect(taskCard).toBeVisible();
  });
});
