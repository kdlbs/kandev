import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

useRegularMode();

test.describe("mobile task-create launch prompt preview", () => {
  test("keeps the launch preview inside the touch dialog", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Mobile Launch Preview Workflow",
    );
    const configuredStart = await apiClient.createWorkflowStep(workflow.id, "Backlog", 0, {
      is_start_step: true,
    });
    const autoStart = await apiClient.createWorkflowStep(workflow.id, "In Progress", 1);

    await apiClient.updateWorkflowStep(configuredStart.id, { events: {} });
    await apiClient.updateWorkflowStep(autoStart.id, {
      prompt: "Mobile launch: {{task_prompt}} | {{task_prompt}} | {task_id}",
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      task_create_last_used: {
        repository_id: seedData.repositoryId,
        branch: "main",
        agent_profile_id: seedData.agentProfileId,
        workflow_ids_by_workspace: { [seedData.workspaceId]: seedData.workflowId },
      },
    });

    try {
      await testPage.setViewportSize({ width: 390, height: 844 });
      const mobile = new MobileKanbanPage(testPage);
      await mobile.goto();
      await mobile.mobileFab.tap();

      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      const workflowSelector = dialog.getByTestId("workflow-selector-trigger");
      await workflowSelector.tap();
      await testPage
        .getByRole("button", { name: /^Mobile Launch Preview Workflow/ })
        .last()
        .tap();
      await dialog.getByTestId("task-description-input").fill("");
      const launchStep = dialog.getByTestId("task-create-launch-step");
      await expect(launchStep).toHaveText("Start step: Backlog");
      await expect(workflowSelector).not.toContainText("Start step:");
      const selectorBox = await workflowSelector.boundingBox();
      const launchStepBox = await launchStep.boundingBox();
      const launchStepInfo = dialog.getByTestId("task-create-launch-step-info");
      await expect(launchStepInfo).toHaveAttribute("aria-label", "Learn about the task start step");
      const launchStepInfoBox = await launchStepInfo.boundingBox();
      if (!selectorBox || !launchStepInfoBox || !launchStepBox) {
        throw new Error("launch step controls have no layout box");
      }
      expect(launchStepInfoBox.x).toBeGreaterThanOrEqual(selectorBox.x + selectorBox.width - 1);
      expect(launchStepBox.x).toBeGreaterThanOrEqual(
        launchStepInfoBox.x + launchStepInfoBox.width - 1,
      );
      expect(launchStepInfoBox.width).toBeGreaterThanOrEqual(44);

      await dialog.getByTestId("task-title-input").fill("Mobile launch preview");
      const description = "Review mobile launch preview";
      await dialog.getByTestId("task-description-input").fill(description);
      await expect(launchStep).toHaveText("Start step: In Progress");

      const toggle = dialog.getByTestId("task-create-launch-preview-toggle");
      await expect(toggle).toHaveAttribute(
        "aria-label",
        "Preview launch prompt with workflow step prompt: In Progress",
      );
      await expect(toggle).toHaveAttribute("aria-pressed", "false");
      const toggleBox = await toggle.boundingBox();
      if (!toggleBox) throw new Error("launch preview toggle has no layout box");
      expect(toggleBox.width).toBeGreaterThanOrEqual(44);
      expect(toggleBox.height).toBeGreaterThanOrEqual(44);

      await toggle.tap();
      const preview = dialog.getByTestId("task-create-launch-preview-content");
      await expect(preview).toContainText(
        `Mobile launch: ${description} | {{task_prompt}} | {task_id}`,
      );
      await expect(dialog.getByTestId("task-description-input")).toHaveCount(0);
      const dialogBox = await dialog.boundingBox();
      const previewBox = await preview.boundingBox();
      if (!dialogBox || !previewBox) throw new Error("launch preview has no layout box");
      expect(previewBox.x).toBeGreaterThanOrEqual(dialogBox.x);
      expect(previewBox.x + previewBox.width).toBeLessThanOrEqual(
        dialogBox.x + dialogBox.width + 1,
      );

      await toggle.tap();
      await expect(dialog.getByTestId("task-description-input")).toHaveValue(description);
      await expect(toggle).toHaveAttribute("aria-pressed", "false");
      const pageWidth = await testPage.evaluate(() => ({
        scroll: document.documentElement.scrollWidth,
        client: document.documentElement.clientWidth,
      }));
      expect(pageWidth.scroll).toBeLessThanOrEqual(pageWidth.client);

      await dialog.getByRole("button", { name: "Cancel", exact: true }).tap();
      await expect(dialog).not.toBeVisible();
    } finally {
      await apiClient.deleteWorkflow(workflow.id).catch(() => {});
    }
  });
});
