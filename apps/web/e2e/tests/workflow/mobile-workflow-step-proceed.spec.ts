import { expect, test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import {
  BOARD_CONTEXT_TASK_TITLE,
  FEATURE_TASK_TITLE,
  seedCrossWorkflowProceedScenario,
} from "./workflow-cross-workflow-proceed-helpers";

test("uses the task workflow when another board is selected on a phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const scenario = await seedCrossWorkflowProceedScenario(apiClient, seedData);
  const kanban = new KanbanPage(testPage);
  await kanban.goto();

  const boardTaskCard = kanban.taskCardByTitle(BOARD_CONTEXT_TASK_TITLE);
  await expect(boardTaskCard).toBeVisible();
  await expect(kanban.taskCardByTitle(FEATURE_TASK_TITLE)).not.toBeVisible();
  await boardTaskCard.tap();

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  const taskDrawer = testPage.getByRole("dialog", { name: "Tasks" });
  const featureTaskRow = taskDrawer
    .getByTestId("sidebar-task-item")
    .filter({ hasText: FEATURE_TASK_TITLE });
  await expect(featureTaskRow).toBeVisible({ timeout: 15_000 });
  await featureTaskRow.tap();

  await expect(testPage).toHaveURL(new RegExp(`/t/${scenario.featureTask.id}(?:\\?|$)`));
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  await expect(session.proceedNextStepButton()).toContainText("Implement");
  await session.proceedNextStepButton().tap();

  await expect
    .poll(async () => (await apiClient.getTask(scenario.featureTask.id)).workflow_step_id, {
      timeout: 15_000,
    })
    .toBe(scenario.implementStep.id);
});
