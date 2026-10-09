import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Sidebar workflow completion icons", () => {
  test("distinguishes a finished turn from a completed workflow", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Completion icon workflow",
    );
    try {
      const workStep = await apiClient.createWorkflowStep(workflow.id, "In Progress", 0);
      const finalStep = await apiClient.createWorkflowStep(
        workflow.id,
        "Completion icon final step",
        1,
      );
      const turnFinishedTask = await apiClient.seedTask(
        seedData.workspaceId,
        "Sidebar turn finished",
        {
          workflow_id: workflow.id,
          workflow_step_id: workStep.id,
          state: "REVIEW",
        },
      );
      const workflowCompleteTask = await apiClient.seedTask(
        seedData.workspaceId,
        "Sidebar workflow complete",
        {
          workflow_id: workflow.id,
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

      // The row can render from the task snapshot before the workspace workflow
      // step snapshot has arrived. Re-drive hydration until the final-step map
      // is present; checking the row alone does not prove that map is ready.
      await expect
        .poll(
          async () => {
            const icon = workflowCompleteRow.getByTestId("task-state-workflow-complete");
            if (await icon.isVisible().catch(() => false)) return true;
            await testPage.reload();
            await session.waitForLoad();
            return icon.isVisible().catch(() => false);
          },
          {
            timeout: 30_000,
            message: "workflow completion icon should follow workflow step hydration",
          },
        )
        .toBe(true);

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
    } finally {
      await apiClient.deleteWorkflow(workflow.id);
    }
  });
});
