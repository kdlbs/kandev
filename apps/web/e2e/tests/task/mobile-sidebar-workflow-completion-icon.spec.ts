import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile sidebar workflow completion icons", () => {
  test("keeps the finished-turn distinction in the phone task drawer", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Sidebar completion icon workflow",
      "simple",
    );
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const sortedSteps = steps.sort((a, b) => a.position - b.position);
    const startStep = sortedSteps.find((step) => step.is_start_step) ?? sortedSteps[0];
    const finalSeedStep = sortedSteps.at(-1);
    if (!startStep || !finalSeedStep) throw new Error("completion icon workflow has no steps");

    const finalStep = await apiClient.createWorkflowStep(
      workflow.id,
      "Mobile completion icon final step",
      finalSeedStep.position + 1,
    );
    const turnFinishedTask = await apiClient.seedTask(
      seedData.workspaceId,
      "Mobile sidebar turn finished",
      {
        workflow_id: workflow.id,
        workflow_step_id: startStep.id,
        state: "REVIEW",
      },
    );
    const workflowCompleteTask = await apiClient.seedTask(
      seedData.workspaceId,
      "Mobile sidebar workflow complete",
      {
        workflow_id: workflow.id,
        workflow_step_id: finalStep.id,
        state: "REVIEW",
      },
    );

    await testPage.goto(`/t/${workflowCompleteTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-task-picker-trigger").tap();

    const drawer = testPage.getByRole("dialog", { name: "Tasks" });
    const turnFinishedRow = drawer.getByTestId("sidebar-task-item").filter({
      hasText: "Mobile sidebar turn finished",
    });
    const workflowCompleteRow = drawer.getByTestId("sidebar-task-item").filter({
      hasText: "Mobile sidebar workflow complete",
    });
    await expect(turnFinishedRow).toBeVisible();
    await expect(workflowCompleteRow).toBeVisible();
    await expect(turnFinishedRow.getByTestId("task-state-turn-finished")).toBeVisible();
    await expect(workflowCompleteRow.getByTestId("task-state-workflow-complete")).toBeVisible();
    await expect(turnFinishedRow.getByTestId("task-state-workflow-complete")).toHaveCount(0);
    await expect(workflowCompleteRow.getByTestId("task-state-turn-finished")).toHaveCount(0);

    expect(workflowCompleteTask.task_id).not.toBe(turnFinishedTask.task_id);
    await prCapture.screenshot("mobile-sidebar-workflow-completion-icons", {
      caption: "Mobile task drawer distinguishes a finished turn from workflow completion.",
    });
  });
});
