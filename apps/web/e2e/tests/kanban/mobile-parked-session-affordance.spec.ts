import { expect, test } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { injectParkedBoardTask } from "./parked-session-affordance-helpers";

test.describe("Mobile parked-session affordance", () => {
  test("shows the parked background-running icon on the board card", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile Parked Board Card Test", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    const card = mobile.taskCard(task.id);
    await expect(card).toBeVisible({ timeout: 10_000 });

    await injectParkedBoardTask(testPage, seedData.workflowId, task.id);

    await expect(card.getByTestId("task-state-background-running")).toBeVisible({
      timeout: 5_000,
    });
    await expect(card.getByTestId("task-state-waiting-for-input")).not.toBeVisible();
  });
});
