import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { expect, type Locator } from "@playwright/test";

export async function seedNavigationTaskPanel(apiClient: ApiClient, seed: SeedData) {
  const tasks = [];
  for (const state of ["TODO", "IN_PROGRESS", "REVIEW", "COMPLETED"]) {
    tasks.push(
      await apiClient.seedTask(
        seed.workspaceId,
        `${state}: improve navigation accessibility and keyboard focus with a deliberately long task title`,
        {
          workflow_id: seed.workflowId,
          workflow_step_id: seed.startStepId,
          state,
        },
      ),
    );
  }
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seed.workspaceId,
      active_view_id: "navigation-states",
      views: [
        {
          id: "navigation-states",
          name: "By status",
          filters: [],
          group: "state",
          sort: { key: "state", direction: "asc" },
          collapsed_groups: [],
        },
      ],
      draft: null,
    },
  });
  return tasks;
}

export async function seedWorkflowGroupPanel(apiClient: ApiClient, seed: SeedData) {
  const options = { workflow_id: seed.workflowId, workflow_step_id: seed.startStepId };
  const parent = await apiClient.seedTask(seed.workspaceId, "Improve empty states", options);
  const child = await apiClient.seedTask(seed.workspaceId, "Check recovery actions", {
    ...options,
    parent_id: parent.task_id,
  });
  const otherStep = seed.steps.find((step) => step.id !== seed.startStepId)!;
  const other = await apiClient.seedTask(seed.workspaceId, "Review keyboard navigation", {
    ...options,
    workflow_step_id: otherStep.id,
  });
  await apiClient.saveUserSettings({
    sidebar_view_state: {
      workspace_id: seed.workspaceId,
      active_view_id: "workflow-hierarchy",
      views: [
        {
          id: "workflow-hierarchy",
          name: "Workflow steps",
          filters: [],
          group: "workflowStep",
          sort: { key: "updatedAt", direction: "desc" },
          collapsed_groups: [],
        },
      ],
      draft: null,
    },
  });
  return { parent, child, other };
}

export async function expectWorkflowGroupHierarchy(group: Locator) {
  const header = group.getByTestId("sidebar-group-header");
  const parent = group.getByTestId("sidebar-task-item").filter({ hasText: "Improve empty states" });
  const child = group
    .getByTestId("sidebar-task-item")
    .filter({ hasText: "Check recovery actions" });
  await expect(parent).toBeVisible();
  const headerBox = (await header.boundingBox())!;
  const parentBox = (await parent.boundingBox())!;
  expect(parentBox.x - headerBox.x).toBeGreaterThanOrEqual(20);
  const parentTitle = (await parent
    .getByText("Improve empty states", { exact: true })
    .boundingBox())!;
  const childTitle = (await child
    .getByText("Check recovery actions", { exact: true })
    .boundingBox())!;
  expect(childTitle.x - parentTitle.x).toBeGreaterThanOrEqual(20);
  const body = group.getByRole("group");
  await expect(body).toHaveAttribute("id", (await header.getAttribute("aria-controls"))!);
  await expect(body).toHaveAttribute("aria-labelledby", (await header.getAttribute("id"))!);
}
