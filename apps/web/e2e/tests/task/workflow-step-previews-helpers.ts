import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

export function workflowStepsResponse(page: Page, workflowId: string) {
  return page.waitForResponse((response) =>
    response.url().includes("/api/v1/workflows/" + workflowId + "/workflow/steps"),
  );
}

export async function expectStepsInOrder(page: Page, workflowId: string, stepNames: string[]) {
  const group = page.getByTestId("workflow-option-steps-" + workflowId);
  await expect(group).toBeVisible();
  const text = (await group.textContent()) ?? "";
  let previousPosition = -1;
  for (const name of stepNames) {
    const position = text.indexOf(name);
    expect(position).toBeGreaterThan(previousPosition);
    previousPosition = position;
  }
}

export type WorkflowStepPreviewScenario = {
  taskId: string;
  kanban: { id: string; stepNames: string[] };
  feature: { id: string; stepNames: string[]; unbrokenStepName: string };
  review: { id: string; stepNames: string[] };
};

export async function seedWorkflowStepPreviewScenario(
  apiClient: ApiClient,
  workspaceId: string,
): Promise<WorkflowStepPreviewScenario> {
  const kanban = await apiClient.createWorkflow(workspaceId, "Preview Kanban");
  const feature = await apiClient.createWorkflow(workspaceId, "Feature Plan");
  const review = await apiClient.createWorkflow(workspaceId, "Contributor Review");
  const unbrokenStepName = "workflow-reference-" + "x".repeat(100);
  const featureSteps = [
    "Analysis",
    unbrokenStepName,
    ...Array.from(
      { length: 40 },
      (_, index) =>
        "Implementation checkpoint " +
        (index + 1) +
        " with a long review note and enough detail to wrap on a phone",
    ),
    "Review",
    "Done",
  ];
  const workflowSeeds = [
    {
      workflow: kanban,
      stepNames: ["Backlog", "In Progress", "Review", "Done"],
    },
    { workflow: feature, stepNames: featureSteps },
    {
      workflow: review,
      stepNames: ["Triage", "Prepare contribution", "Maintainer review", "Complete"],
    },
  ];

  for (const { workflow, stepNames } of workflowSeeds) {
    for (const [position, name] of stepNames.entries()) {
      await apiClient.createWorkflowStep(workflow.id, name, position, {
        is_start_step: position === 0,
      });
    }
  }

  const kanbanSteps = await apiClient.listWorkflowSteps(kanban.id);
  const startStepId = kanbanSteps.steps.find((step) => step.name === "Backlog")?.id;
  if (!startStepId) throw new Error("Preview Kanban has no Backlog step");
  const task = await apiClient.seedTask(workspaceId, "Workflow preview navigation task", {
    workflow_id: kanban.id,
    workflow_step_id: startStepId,
  });

  await apiClient.saveUserSettings({
    workspace_id: workspaceId,
    workflow_filter_id: kanban.id,
    task_create_last_used: {
      workflow_ids_by_workspace: { [workspaceId]: kanban.id },
    },
  });

  return {
    taskId: task.task_id,
    kanban: { id: kanban.id, stepNames: ["Backlog", "In Progress", "Review", "Done"] },
    feature: { id: feature.id, stepNames: featureSteps, unbrokenStepName },
    review: {
      id: review.id,
      stepNames: ["Triage", "Prepare contribution", "Maintainer review", "Complete"],
    },
  };
}

export async function cleanupWorkflowStepPreviewScenario(
  apiClient: ApiClient,
  scenario: WorkflowStepPreviewScenario,
): Promise<void> {
  for (const workflowId of [scenario.review.id, scenario.feature.id, scenario.kanban.id]) {
    await apiClient.deleteWorkflow(workflowId).catch(() => {});
  }
}
