import { expect, test } from "../../fixtures/test-base";
import { waitForLatestSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

test("shows the current agent status in the existing phone Move to drawer", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile workflow step progress",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await waitForLatestSessionDone(apiClient, task.id, 1, "workflow step progress agent turn");
  const settledTask = await apiClient.getTask(task.id);

  await testPage.setViewportSize({ width: 360, height: 780 });
  await testPage.goto(`/t/${task.id}`);
  await new SessionPage(testPage).waitForLoad();
  await testPage.getByTestId("mobile-task-picker-trigger").tap();

  const taskSheet = testPage.getByRole("dialog", { name: "Tasks" });
  const taskRow = taskSheet
    .getByTestId("sidebar-task-item")
    .filter({ hasText: "Mobile workflow step progress" });
  await taskRow.getByRole("button", { name: "Task actions" }).tap();
  await testPage.getByTestId("task-context-move-to").tap();

  const currentStep = testPage.getByTestId(`task-context-step-${settledTask.workflow_step_id}`);
  await expect(currentStep).toBeVisible();
  await expect(
    currentStep.getByTestId(`task-context-step-progress-${settledTask.workflow_step_id}`),
  ).toContainText("Waiting for input");
  await expect(currentStep).toContainText("Current");
  await expect(currentStep).toHaveCSS("min-height", "44px");
});
