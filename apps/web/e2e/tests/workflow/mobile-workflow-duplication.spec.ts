import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";
import { seedWorkflowDuplication } from "./workflow-duplication-helpers";

test.describe("mobile: workflow duplication", () => {
  test("duplicates and saves a workflow through the touch action", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const seed = await seedWorkflowDuplication(
      apiClient,
      seedData.workspaceId,
      "Mobile Duplication Source",
      seedData.agentProfileId,
    );
    const before = await apiClient.listWorkflows(seedData.workspaceId);
    const settings = new WorkflowSettingsPage(testPage);

    await settings.goto(seedData.workspaceId);
    const sourceCard = await settings.findWorkflowCard("Mobile Duplication Source");
    const duplicateButton = settings.duplicateWorkflowButton(sourceCard);
    await expect(duplicateButton).toBeVisible();
    await expect(duplicateButton).toBeEnabled();
    const duplicateBox = await duplicateButton.boundingBox();
    expect(duplicateBox).not.toBeNull();
    expect(duplicateBox!.height).toBeGreaterThanOrEqual(44);

    await settings.duplicateWorkflow(sourceCard, true);
    const copyCard = await settings.findWorkflowCard("Mobile Duplication Source (copy)", {
      waitForName: true,
    });
    await expect(copyCard).toBeVisible();
    await expect(settings.floatingSave).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);

    const beforeSave = await apiClient.listWorkflows(seedData.workspaceId);
    expect(beforeSave.workflows).toHaveLength(before.workflows.length);

    await settings.saveChanges(true);
    await testPage.reload();
    const reloadedCopy = await settings.findWorkflowCard("Mobile Duplication Source (copy)", {
      waitForName: true,
    });
    await expect(reloadedCopy).toBeVisible();

    const afterSave = await apiClient.listWorkflows(seedData.workspaceId);
    const copy = afterSave.workflows.find(
      (workflow) => workflow.name === "Mobile Duplication Source (copy)",
    );
    expect(copy).toBeDefined();
    const copiedSteps = await apiClient.listWorkflowSteps(copy!.id);
    expect(copiedSteps.steps).toHaveLength(2);
    expect(copiedSteps.steps.map((step) => step.id)).not.toContain(seed.reviewStepId);
    expect(copiedSteps.steps.map((step) => step.id)).not.toContain(seed.doneStepId);

    const tasks = await apiClient.listTasks(seedData.workspaceId);
    expect(tasks.tasks.find((task) => task.id === seed.taskId)?.workflow_step_id).toBe(
      seed.reviewStepId,
    );
    await assertNoDocumentHorizontalOverflow(testPage);
  });
});
