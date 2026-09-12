import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

// docs/specs/tasks/requirements/board-priority-sort-filter-view-state.md
// REQ-003: the priority filter and board sort control must be reachable and
// operable on the phone surface, through the mobile menu sheet rather than
// the desktop dropdown, and a selection made there must be the same
// persisted value the desktop surface reads.
const TASK_CRITICAL = "Mobile sort task critical";
const TASK_MEDIUM = "Mobile sort task medium";

async function openMobileMenu(testPage: import("@playwright/test").Page) {
  await testPage.getByRole("button", { name: "Open menu" }).click();
  await testPage.getByTestId("mobile-home-menu-card").waitFor({ state: "visible" });
}

async function closeMobileMenu(testPage: import("@playwright/test").Page) {
  const card = testPage.getByTestId("mobile-home-menu-card");
  await expect(async () => {
    if ((await card.count()) > 0) {
      await testPage.keyboard.press("Escape");
    }
    await expect(card).toHaveCount(0, { timeout: 1_000 });
  }).toPass({ timeout: 15_000 });
}

test.describe("Mobile board priority sort and filter", () => {
  test.afterEach(async ({ apiClient, seedData }) => {
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      kanban_sort: "created_desc",
      kanban_priority_filter_tokens: [],
    });
  });

  test("the priority filter is reachable from the mobile menu sheet and narrows the board", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    await apiClient.createTask(seedData.workspaceId, TASK_CRITICAL, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      priority: "critical",
    });
    await apiClient.createTask(seedData.workspaceId, TASK_MEDIUM, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      priority: "medium",
    });

    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCardByTitle(TASK_CRITICAL)).toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCardByTitle(TASK_MEDIUM)).toBeVisible({ timeout: 10_000 });

    await openMobileMenu(testPage);
    const criticalOption = testPage.getByTestId("mobile-priority-filter-option-critical");
    await expect(criticalOption).toBeVisible();
    await criticalOption.click();
    await closeMobileMenu(testPage);

    await expect(kanban.taskCardByTitle(TASK_CRITICAL)).toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCardByTitle(TASK_MEDIUM)).not.toBeVisible();
  });

  test("a board sort selected on desktop settings is in effect on mobile without being re-made", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    await apiClient.createTask(seedData.workspaceId, TASK_CRITICAL, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      priority: "critical",
    });
    await apiClient.createTask(seedData.workspaceId, TASK_MEDIUM, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      priority: "medium",
    });

    // Set both values the way the desktop surface would persist them, then
    // load the phone surface fresh — AC-003.2: a selection made at one
    // breakpoint is in effect at the other without being re-made.
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      kanban_sort: "priority_desc",
      kanban_priority_filter_tokens: ["critical"],
    });

    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();

    await expect(kanban.taskCardByTitle(TASK_CRITICAL)).toBeVisible({ timeout: 10_000 });
    await expect(kanban.taskCardByTitle(TASK_MEDIUM)).not.toBeVisible();

    await openMobileMenu(testPage);
    await expect(testPage.getByTestId("mobile-board-sort")).toContainText("Priority");
    await expect(testPage.getByTestId("mobile-priority-filter-option-critical")).toHaveAttribute(
      "data-state",
      "checked",
    );
    await closeMobileMenu(testPage);
  });
});
