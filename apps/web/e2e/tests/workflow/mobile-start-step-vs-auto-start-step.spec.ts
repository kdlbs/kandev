import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

test.describe("mobile start step vs auto-start step", () => {
  test("starts a plan-mode task on the first auto-start step", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Mobile Plan Mode Routing",
    );
    const backlog = await apiClient.createWorkflowStep(workflow.id, "Backlog", 0, {
      is_start_step: true,
    });
    const inProgress = await apiClient.createWorkflowStep(workflow.id, "In Progress", 1);
    await apiClient.updateWorkflowStep(backlog.id, { events: {} });
    await apiClient.updateWorkflowStep(inProgress.id, {
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      task_create_last_used: {
        repository_id: seedData.repositoryId,
        branch: "main",
        agent_profile_id: seedData.agentProfileId,
        workflow_ids_by_workspace: { [seedData.workspaceId]: workflow.id },
      },
      enable_preview_on_click: false,
    });

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.tap();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByTestId("task-title-input").fill("Mobile plan mode routing");
    await dialog.getByTestId("task-description-input").fill("/e2e:simple-message");

    const planModeButton = dialog.getByTestId("mobile-plan-mode");
    await expect(planModeButton).toBeEnabled({ timeout: 30_000 });
    await planModeButton.tap();
    await expect(dialog).not.toBeVisible({ timeout: 15_000 });
    await expect(testPage).toHaveURL(/\/t\/.*layout=plan/, { timeout: 15_000 });

    const taskId = testPage.url().match(/\/t\/([^/?#]+)/)?.[1];
    expect(taskId).toBeTruthy();
    if (!taskId) throw new Error("Mobile plan-mode task ID was missing from the session URL");

    await expect
      .poll(async () => (await apiClient.getTask(taskId)).workflow_step_id, {
        timeout: 30_000,
        message: "Waiting for the mobile plan-mode task to use the auto-start destination",
      })
      .toBe(inProgress.id);

    await mobile.goto();
    await mobile.boardNavigator.tap();
    const inProgressTab = testPage
      .getByTestId("mobile-board-navigator-drawer")
      .getByTestId("column-tab-1");
    await expect(inProgressTab).toContainText("In Progress");
    await inProgressTab.tap();
    await expect(mobile.taskCardByTitle("Mobile plan mode routing")).toBeVisible({
      timeout: 15_000,
    });
  });
});
