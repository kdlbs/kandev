import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Desktop task-row menu grouping", () => {
  test("orders the groups and persists a priority choice", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const sameWorkflowStep =
      seedData.steps.find((step) => step.id !== seedData.startStepId) ??
      (await apiClient.createWorkflowStep(seedData.workflowId, "Menu grouping second step", 1)).id;
    const targetWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Menu grouping target workflow",
    );
    const targetStep = await apiClient.createWorkflowStep(targetWorkflow.id, "Incoming", 0);
    const task = await apiClient.createTask(seedData.workspaceId, "Menu grouping root task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      priority: "high",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const row = session.sidebarTaskItem(task.title).first();
    await expect(row).toBeVisible();
    await row.click({ button: "right" });

    const menu = testPage.locator('[data-slot="context-menu-content"][data-state="open"]').last();
    await expect(menu).toBeVisible();
    const labels = (await menu.locator(":scope > [role='menuitem']").allTextContents()).map(
      (text) => text.replace(/\s+/g, " ").trim(),
    );
    expect(labels).toEqual([
      "Pin",
      "Color",
      "Priority",
      "Edit",
      "Rename",
      "Duplicate",
      "Create Subtask",
      "Nest under",
      "Link",
      "Move to",
      "Send to workflow",
      "Archive",
      "Delete",
    ]);

    const separators = menu.locator(":scope > [data-slot='context-menu-separator']");
    await expect(separators).toHaveCount(4);
    await expect(menu.getByTestId("task-context-send-to-workflow")).toBeVisible();
    await prCapture.screenshot("desktop-task-row-menu-grouping", {
      caption: "Desktop task-row actions grouped by purpose",
    });

    await menu.getByTestId("task-context-priority").hover();
    const lowOption = testPage.getByTestId("task-context-priority-low");
    await expect(lowOption).toBeVisible();
    await lowOption.click();
    await expect.poll(async () => (await apiClient.getTask(task.id)).priority).toBe("low");

    // Keep the move fixtures live in the test so the grouping assertion proves
    // both movement entries are admitted, not just the same-workflow branch.
    expect(sameWorkflowStep).toBeTruthy();
    expect(targetStep.id).toBeTruthy();
  });
});
