import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workflow settings on mobile", () => {
  test("configures an all child tasks complete transition", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Mobile Child Completion Settings",
    );
    const waitStep = await apiClient.createWorkflowStep(workflow.id, "Waiting", 0);
    await apiClient.createWorkflowStep(workflow.id, "Done", 1);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("Mobile Child Completion Settings");
    await expect(card).toBeVisible();
    await page.stepNodeByName(card, "Waiting").click();

    const childCompletionSelect = card.getByTestId(
      `${waitStep.id}-children-completed-transition-select`,
    );
    await expect(childCompletionSelect).toBeVisible();
    await childCompletionSelect.click();
    await testPage.getByRole("option", { name: "Move to next step" }).click();

    await page.saveButton(card).click();
    await testPage.waitForTimeout(1000);

    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const savedWaitStep = steps.find((step) => step.id === waitStep.id);
    expect(savedWaitStep?.events?.on_children_completed).toEqual([{ type: "move_to_next" }]);
  });
});
