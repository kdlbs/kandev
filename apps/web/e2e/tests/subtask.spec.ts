import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

test.describe("Subtask support", () => {
  test("subtask badge visible on kanban card", async ({ testPage, apiClient, seedData }) => {
    // Create parent task via API
    const parent = await apiClient.createTask(seedData.workspaceId, "Parent Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    // Create subtask via API with parent_id
    await apiClient.createTask(seedData.workspaceId, "Child Subtask", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.task_id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // Parent card is visible but does NOT have "Subtask" badge
    const parentCard = kanban.taskCardByTitle("Parent Task");
    await expect(parentCard).toBeVisible({ timeout: 10_000 });
    await expect(parentCard.getByText("Subtask")).not.toBeVisible();

    // Subtask card is visible and HAS "Subtask" badge
    const subtaskCard = kanban.taskCardByTitle("Child Subtask");
    await expect(subtaskCard).toBeVisible({ timeout: 10_000 });
    await expect(subtaskCard.getByText("Subtask")).toBeVisible();
  });

  test("create subtask from sidebar header button", async ({ testPage, apiClient, seedData }) => {
    // Create a task with an agent so we have a session to navigate to
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Subtask Parent",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Navigate to the session page
    testPage.goto(`/s/${task.session_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for agent to complete
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Click the "+ Subtask" button in the dockview sidebar header
    const subtaskBtn = testPage.getByTestId("new-subtask-button");
    await expect(subtaskBtn).toBeVisible({ timeout: 5_000 });
    await subtaskBtn.click();

    // The compact NewSubtaskDialog should open with pre-filled title containing random hex suffix
    const titleInput = testPage.getByTestId("subtask-title-input");
    await expect(titleInput).toBeVisible({ timeout: 5_000 });
    await expect(titleInput).toHaveValue(/Subtask Parent #[0-9a-f]{4}/);

    // Fill prompt and submit
    const promptInput = testPage.getByTestId("subtask-prompt-input");
    await expect(promptInput).toBeVisible();
    await promptInput.fill("/e2e:simple-message");

    const submitBtn = testPage.getByRole("button", { name: "Create Subtask" });
    await submitBtn.click();
    await expect(titleInput).not.toBeVisible({ timeout: 10_000 });

    // After creation, we navigate to the new subtask's session
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    // Verify the subtask card appears on the kanban board with "Subtask" badge
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const subtaskCard = kanban.taskCardByTitle(/Subtask Parent #[0-9a-f]{4}/);
    await expect(subtaskCard).toBeVisible({ timeout: 10_000 });
    await expect(subtaskCard.getByText("Subtask")).toBeVisible();
  });
});
