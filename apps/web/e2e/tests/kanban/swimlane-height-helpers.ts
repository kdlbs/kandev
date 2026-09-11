import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

export async function withHeightWorkflows(
  api: ApiClient,
  seed: SeedData,
  run: (second: {
    workflowId: string;
    stepId: string;
    taskId: string;
    firstTaskId: string;
    stepIndex: number;
  }) => Promise<void>,
) {
  const { settings } = await api.getUserSettings();
  const workflow = await api.createWorkflow(seed.workspaceId, "Compact second", "simple");
  try {
    const { steps } = await api.listWorkflowSteps(workflow.id);
    const step = steps.find((item) => item.is_start_step) ?? steps[0];
    const first = await api.createTask(seed.workspaceId, "Sparse first card", {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
    });
    const second = await api.createTask(seed.workspaceId, "Sparse second card", {
      workflow_id: workflow.id,
      workflow_step_id: step.id,
    });
    await api.saveUserSettings({ workflow_filter_id: "", repository_ids: [] });
    await run({
      workflowId: workflow.id,
      stepId: step.id,
      taskId: second.id,
      firstTaskId: first.id,
      stepIndex: steps.findIndex((item) => item.id === step.id),
    });
  } finally {
    await api.deleteWorkflow(workflow.id);
    const restored = await api.rawRequest("PATCH", "/api/v1/user/settings", {
      workflow_filter_id: settings.workflow_filter_id ?? "",
      repository_ids: settings.repository_ids ?? [],
      enable_preview_on_click: settings.enable_preview_on_click ?? false,
      kanban_hidden_step_ids: settings.kanban_hidden_step_ids ?? {},
      workflow_ids_with_auto_hide_empty_steps:
        settings.workflow_ids_with_auto_hide_empty_steps ?? [],
      kanban_view_mode: settings.kanban_view_mode ?? "",
    });
    expect(restored.ok).toBe(true);
  }
}

export async function expectCompactColumn(column: Locator) {
  await expect
    .poll(async () => (await column.boundingBox())?.height ?? 0)
    .toBeGreaterThanOrEqual(200);
  await expect.poll(async () => (await column.boundingBox())!.height).toBeLessThanOrEqual(400);
}

export async function selectHeightWorkflow(page: Page, name: string) {
  await page.getByTestId("display-button").click();
  await page.getByTestId("display-workflow-filter").click();
  await page.getByRole("listbox").getByRole("option", { name, exact: true }).click();
  await page.keyboard.press("Escape");
}

export async function expectNoDocumentOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    ),
  ).toBe(0);
}
