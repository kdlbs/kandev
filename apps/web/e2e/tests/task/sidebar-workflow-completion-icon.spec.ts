import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Sidebar workflow completion icons", () => {
  test("distinguishes a finished turn from a completed workflow", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const finalSeedStep = seedData.steps.at(-1);
    if (!finalSeedStep) throw new Error("seed workflow has no steps");

    const finalStep = await apiClient.createWorkflowStep(
      seedData.workflowId,
      "Completion icon final step",
      finalSeedStep.position + 1,
    );
    const turnFinishedTask = await apiClient.seedTask(
      seedData.workspaceId,
      "Sidebar turn finished",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        state: "REVIEW",
      },
    );
    const workflowCompleteTask = await apiClient.seedTask(
      seedData.workspaceId,
      "Sidebar workflow complete",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: finalStep.id,
        state: "REVIEW",
      },
    );

    await testPage.goto(`/t/${workflowCompleteTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const turnFinishedRow = session.sidebarTaskItem("Sidebar turn finished");
    const workflowCompleteRow = session.sidebarTaskItem("Sidebar workflow complete");
    await expect(turnFinishedRow).toBeVisible();
    await expect(workflowCompleteRow).toBeVisible();
    await expect(turnFinishedRow.getByTestId("task-state-turn-finished")).toBeVisible();
    await expect(turnFinishedRow.getByTestId("task-state-workflow-complete")).toHaveCount(0);
    await expect(workflowCompleteRow.getByTestId("task-state-workflow-complete")).toBeVisible();
    await expect(workflowCompleteRow.getByTestId("task-state-turn-finished")).toHaveCount(0);

    // Keep the created task referenced so the route setup remains explicit if the fixture
    // changes its task cleanup behavior.
    expect(workflowCompleteTask.task_id).not.toBe(turnFinishedTask.task_id);
    await prCapture.screenshot("desktop-sidebar-workflow-completion-icons", {
      caption: "Desktop task sidebar distinguishes a finished turn from workflow completion.",
    });
  });
});
