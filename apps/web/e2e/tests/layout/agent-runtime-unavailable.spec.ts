import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import {
  setAgentRuntimeAvailability,
  stubAgentRuntimeRestart,
} from "../../helpers/agent-runtime-availability";
import { KanbanPage } from "../../pages/kanban-page";

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      features: Record<string, boolean>;
      setFeatures: (features: Record<string, boolean>) => void;
    };
  };
};

async function setAppStatusBarEnabled(page: Page, enabled: boolean): Promise<void> {
  await page.evaluate((nextEnabled) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge missing");
    const state = store.getState();
    state.setFeatures({ ...state.features, appStatusBar: nextEnabled });
  }, enabled);
}

test.describe("Agent runtime availability", () => {
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

    await setAppStatusBarEnabled(testPage, false);
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
