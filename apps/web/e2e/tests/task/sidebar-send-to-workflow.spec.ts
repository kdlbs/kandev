import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

test.describe("Task sidebar send to workflow", () => {
  test("right-click sends a sidebar task to another workflow without navigation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const targetWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Sidebar Target Workflow",
    );
    const targetStep = await apiClient.createWorkflowStep(targetWorkflow.id, "Sidebar Incoming", 0);
    const anchor = await apiClient.createTask(seedData.workspaceId, "Sidebar Anchor Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.createTask(seedData.workspaceId, "Sidebar Send Candidate", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await testPage.goto(`/t/${anchor.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebarTaskItem("Sidebar Send Candidate")).toBeVisible({
      timeout: 15_000,
    });
    const beforeUrl = testPage.url();

    await session.sendSidebarTaskToWorkflow(
      "Sidebar Send Candidate",
      targetWorkflow.id,
      targetStep.id,
    );

    await expect(testPage.getByText(/Moved task to/i)).toBeVisible({ timeout: 10_000 });
    expect(testPage.url()).toBe(beforeUrl);

    const kanban = new KanbanPage(testPage);
    await testPage.goto(`/?workflowId=${targetWorkflow.id}`);
    await expect(kanban.board).toBeVisible();
    await expect(kanban.taskCardInColumn("Sidebar Send Candidate", targetStep.id)).toBeVisible({
      timeout: 10_000,
    });

    await testPage.reload();
    await expect(kanban.taskCardInColumn("Sidebar Send Candidate", targetStep.id)).toBeVisible({
      timeout: 10_000,
    });
  });
});
