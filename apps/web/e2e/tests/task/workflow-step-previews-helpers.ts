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

export async function expectUnbrokenStepToFitGroup(
  page: Page,
  workflowId: string,
  stepName: string,
) {
  const text = page
    .getByTestId("workflow-option-steps-" + workflowId)
    .getByText(stepName, { exact: true });
  await expect(text).toBeVisible();
  const layout = await text.evaluate((element) => {
    const group = element.closest<HTMLElement>("[data-testid^='workflow-option-steps-']");
    if (!group) throw new Error("Workflow step text is outside its step group");
    const range = document.createRange();
    range.selectNodeContents(element);
    const groupBox = group.getBoundingClientRect();
    return {
      group: {
        left: groupBox.left,
        right: groupBox.right,
      },
      text: Array.from(range.getClientRects(), (rect) => ({
        left: rect.left,
        right: rect.right,
      })),
    };
  });
  expect(layout.text.length).toBeGreaterThan(1);
  for (const line of layout.text) {
    expect(line.left).toBeGreaterThanOrEqual(layout.group.left - 1);
    expect(line.right).toBeLessThanOrEqual(layout.group.right + 1);
  }
}

export type WorkflowStepPreviewScenario = {
  taskId: string;
  kanban: { id: string; stepNames: string[] };
  feature: { id: string; stepNames: string[]; unbrokenStepName: string };
  review: { id: string; stepNames: string[] };
  extraWorkflows: Array<{ id: string; stepName: string }>;
};

type WorkflowStepPreviewScenarioOptions = {
  extraWorkflowCount?: number;
};

export async function seedWorkflowStepPreviewScenario(
  apiClient: ApiClient,
  workspaceId: string,
  { extraWorkflowCount = 0 }: WorkflowStepPreviewScenarioOptions = {},
): Promise<WorkflowStepPreviewScenario> {
  const kanban = await apiClient.createWorkflow(workspaceId, "Preview Kanban");
  const feature = await apiClient.createWorkflow(workspaceId, "Feature Plan");
  const review = await apiClient.createWorkflow(workspaceId, "Contributor Review");
  const unbrokenStepName = "workflow-reference-" + "x".repeat(100);
  const featureSteps = [
    "Analysis",
    unbrokenStepName,
    "Implementation checkpoint with a long review note and enough detail to wrap on a phone",
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

  const extraWorkflows: Array<{ id: string; stepName: string }> = [];
  for (let index = 0; index < extraWorkflowCount; index += 1) {
    const workflow = await apiClient.createWorkflow(workspaceId, `Picker overflow ${index + 1}`);
    const stepName = `Overflow start ${index + 1}`;
    await apiClient.createWorkflowStep(workflow.id, stepName, 0, {
      is_start_step: true,
    });
    extraWorkflows.push({ id: workflow.id, stepName });
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
    extraWorkflows,
  };
}

export async function cleanupWorkflowStepPreviewScenario(
  apiClient: ApiClient,
  scenario: WorkflowStepPreviewScenario,
): Promise<void> {
  for (const workflowId of [
    ...scenario.extraWorkflows.map(({ id }) => id),
    scenario.review.id,
    scenario.feature.id,
    scenario.kanban.id,
  ]) {
    await apiClient.deleteWorkflow(workflowId).catch(() => {});
  }
}
