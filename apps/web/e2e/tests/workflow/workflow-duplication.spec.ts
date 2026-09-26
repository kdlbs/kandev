import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { promptEditorText } from "../../helpers/settings-prompt-editor";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";
import { seedWorkflowDuplication } from "./workflow-duplication-helpers";

test.describe("workflow duplication", () => {
  test("creates a configured copy only after Save changes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const seed = await seedWorkflowDuplication(
      apiClient,
      seedData.workspaceId,
      "Duplication Source",
      seedData.agentProfileId,
    );
    const before = await apiClient.listWorkflows(seedData.workspaceId);
    const sourceStepsBefore = await apiClient.listWorkflowSteps(seed.workflowId);
    const settings = new WorkflowSettingsPage(testPage);

    await settings.goto(seedData.workspaceId);
    const sourceCard = await settings.findWorkflowCard("Duplication Source");
    await expect(settings.duplicateWorkflowButton(sourceCard)).toBeEnabled();

    await settings.duplicateWorkflow(sourceCard);
    const copyCard = await settings.findWorkflowCard("Duplication Source (copy)", {
      waitForName: true,
    });
    await expect(copyCard).toBeVisible();
    await expect(copyCard.getByTestId("workflow-description-input")).toHaveValue(
      "Copied workflow description",
    );
    await expect(promptEditorText(copyCard.getByTestId("workflow-prompt-input"))).toContainText(
      "Copied workflow prompt",
    );
    await expect(copyCard.getByText("Review", { exact: true })).toBeVisible();
    await expect(copyCard.getByText("Done", { exact: true })).toBeVisible();

    const beforeSave = await apiClient.listWorkflows(seedData.workspaceId);
    expect(beforeSave.workflows).toHaveLength(before.workflows.length);
    expect(
      beforeSave.workflows.some((workflow) => workflow.name === "Duplication Source (copy)"),
    ).toBe(false);

    await settings.saveChanges();

    const afterSave = await apiClient.listWorkflows(seedData.workspaceId);
    const copy = afterSave.workflows.find(
      (workflow) => workflow.name === "Duplication Source (copy)",
    );
    expect(copy).toBeDefined();
    expect(afterSave.workflows).toHaveLength(before.workflows.length + 1);
    expect(copy).toMatchObject({
      description: "Copied workflow description",
      prompt: "Copied workflow prompt",
      agent_profile_id: seedData.agentProfileId,
    });

    const copiedSteps = await apiClient.listWorkflowSteps(copy!.id);
    expect(copiedSteps.steps).toHaveLength(sourceStepsBefore.steps.length);
    const copiedStepIds = copiedSteps.steps.map((step) => step.id);
    for (const sourceStep of sourceStepsBefore.steps) {
      expect(copiedStepIds).not.toContain(sourceStep.id);
    }
    const copiedReview = copiedSteps.steps.find((step) => step.name === "Review");
    const copiedDone = copiedSteps.steps.find((step) => step.name === "Done");
    expect(copiedReview).toBeDefined();
    expect(copiedDone).toBeDefined();
    expect(copiedReview?.stage_type).toBe("review");
    expect(copiedReview?.events?.on_turn_complete).toEqual([
      { type: "move_to_step", config: { step_id: copiedDone!.id } },
    ]);
    expect(copiedDone?.pull_from_step_id).toBe(copiedReview!.id);

    const tasks = await apiClient.listTasks(seedData.workspaceId);
    const sourceTask = tasks.tasks.find((task) => task.id === seed.taskId);
    expect(sourceTask?.workflow_step_id).toBe(seed.reviewStepId);
    expect(copiedSteps.steps.map((step) => step.id)).not.toContain(sourceTask?.workflow_step_id);

    await testPage.reload();
    const reloadedCopy = await settings.findWorkflowCard("Duplication Source (copy)", {
      waitForName: true,
    });
    await expect(reloadedCopy).toBeVisible();
    await expect(reloadedCopy.getByTestId("workflow-description-input")).toHaveValue(
      "Copied workflow description",
    );
    const reloadedSource = await settings.findWorkflowCard("Duplication Source");
    await expect(reloadedSource).toBeVisible();
  });
});
