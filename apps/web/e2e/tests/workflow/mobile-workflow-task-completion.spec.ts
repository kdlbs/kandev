import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow task completion on mobile", () => {
  test("keeps the final-step control and help disclosure touch accessible", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      `Mobile completion editor ${Date.now()}`,
    );
    const first = await apiClient.createWorkflowStep(workflow.id, "Draft", 0, {
      is_start_step: true,
    });
    const final = await apiClient.createWorkflowStep(workflow.id, "Done", 1, {
      complete_task_on_enter: true,
    });

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = await settings.findWorkflowCard(workflow.name, { waitForName: true });
    await settings.selectStep(card, "Draft", true);
    await expect(settings.completeTaskOnEnterCheckbox(card, first.id)).toHaveCount(0);
    await expect(settings.completeTaskOnEnterHelp(card, first.id)).toHaveCount(0);

    await settings.selectStep(card, "Done", true);
    const checkbox = settings.completeTaskOnEnterCheckbox(card, final.id);
    const help = settings.completeTaskOnEnterHelp(card, final.id);
    await expect(checkbox).toBeChecked();
    const helpBox = await help.boundingBox();
    expect(helpBox).not.toBeNull();
    expect(helpBox!.height).toBeGreaterThanOrEqual(44);
    await help.tap();
    await expect(testPage.getByRole("tooltip")).toContainText(
      "Marks the task complete when it enters this final step",
    );
    await expect(checkbox).toBeChecked();

    await checkbox.tap();
    await expect(checkbox).not.toBeChecked();
    const viewportWidth = await testPage.evaluate(() => window.innerWidth);
    const saveButton = settings.floatingSave.getByRole("button", { name: "Save changes" });
    for (const control of [checkbox, help, saveButton]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(viewportWidth);
    }
    await settings.saveChanges(true);
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    expect(steps.find((step) => step.id === final.id)?.complete_task_on_enter).toBe(false);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile workflow task completion");
  });
});
