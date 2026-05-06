import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import type { Page } from "@playwright/test";

const TASK_VISIBLE_TIMEOUT = 10_000;

async function pickListboxOption(page: Page, optionLabel: string): Promise<void> {
  // Radix Select renders the currently-selected item twice (once in the trigger
  // for SelectValue display, once in the listbox). Scope to the listbox so we
  // click the real option.
  const listbox = page.getByRole("listbox");
  await listbox.getByRole("option", { name: optionLabel, exact: true }).click();
  await expect(listbox).toHaveCount(0);
}

async function closeDisplayDropdown(page: Page): Promise<void> {
  const trigger = page.getByTestId("display-button");
  if ((await trigger.getAttribute("data-state")) === "open") {
    await trigger.click({ force: true });
  }
  await expect(trigger).not.toHaveAttribute("data-state", "open");
  await expect(page.getByRole("menu")).toHaveCount(0);
}

async function selectWorkflowFilter(page: Page, optionLabel: string): Promise<void> {
  await page.getByTestId("display-button").click();
  await page.getByTestId("display-workflow-filter").click();
  await pickListboxOption(page, optionLabel);
  await closeDisplayDropdown(page);
}

test.describe("Kanban workflow filter", () => {
  test.afterEach(async ({ apiClient, seedData }) => {
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      repository_ids: [],
    });
  });

  // Regression: c64e835 made resolveDesiredWorkflowId fall back to the first
  // visible workflow whenever both the active id and persisted setting were
  // null. The kanban page's useWorkflowSelection effect then silently
  // overwrote a freshly-picked "All Workflows" choice on the next render.
  // The /tasks list page does not run that effect, so the existing
  // task-list-filters spec missed this — pin the kanban path explicitly.
  test("'All Workflows' selection persists on the kanban board", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflowB = await apiClient.createWorkflow(seedData.workspaceId, "Workflow B", "simple");
    const stepsB = (await apiClient.listWorkflowSteps(workflowB.id)).steps;
    const startB = stepsB.find((s) => s.is_start_step) ?? stepsB[0];

    await apiClient.createTask(seedData.workspaceId, "Alpha task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Beta task", {
      workflow_id: workflowB.id,
      workflow_step_id: startB.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await expect(testPage.getByText("Alpha task")).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
    await expect(testPage.getByText("Beta task")).not.toBeVisible();

    await selectWorkflowFilter(testPage, "All Workflows");

    // Both tasks visible — useWorkflowSelection must not overwrite the null choice.
    await expect(testPage.getByText("Alpha task")).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });
    await expect(testPage.getByText("Beta task")).toBeVisible({ timeout: TASK_VISIBLE_TIMEOUT });

    // Re-open the dropdown and confirm the trigger still reads "All Workflows"
    // — proves the choice is stable, not just the rendered task list.
    await testPage.getByTestId("display-button").click();
    await expect(testPage.getByTestId("display-workflow-filter")).toContainText("All Workflows");
  });
});
