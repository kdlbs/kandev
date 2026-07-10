import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

const VISIBLE_TIMEOUT = 10_000;

// E2E coverage for the no-cascade-by-default archive / delete behaviour:
// the confirmation dialog must expose an opt-in checkbox when the task
// has live subtasks, default to unchecked (subtasks preserved), and
// propagate the choice to the backend.
test.describe("Kanban card archive — cascade subtasks toggle", () => {
  test("renders the cascade checkbox only when the task has subtasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const lonely = await apiClient.createTask(seedData.workspaceId, "No-Subtasks Parent", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCard(lonely.id)).toBeVisible({ timeout: VISIBLE_TIMEOUT });

    await kanban.openTaskActionsMenu(lonely.id);
    await testPage.getByRole("menuitem", { name: "Archive" }).click();

    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    // No subtasks → no checkbox.
    await expect(testPage.getByTestId("archive-cascade-checkbox")).toHaveCount(0);
  });

  test("archiving the parent without ticking the box leaves subtasks on the board", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const parent = await apiClient.createTask(seedData.workspaceId, "Parent Keep Subs", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Survivor Subtask A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });
    await apiClient.createTask(seedData.workspaceId, "Survivor Subtask B", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("Parent Keep Subs")).toBeVisible({
      timeout: VISIBLE_TIMEOUT,
    });

    await kanban.openTaskActionsMenu(parent.id);
    await testPage.getByRole("menuitem", { name: "Archive" }).click();

    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    // Checkbox is present, displays the count, and is unchecked by default.
    const cascade = testPage.getByTestId("archive-cascade-checkbox");
    await expect(cascade).toBeVisible();
    await expect(dialog).toContainText("Also archive 2 subtasks");
    await expect(cascade).not.toBeChecked();

    await dialog.getByRole("button", { name: "Archive" }).click();

    // Parent leaves the board, subtasks stay.
    await expect(kanban.taskCardByTitle("Parent Keep Subs")).not.toBeVisible({
      timeout: VISIBLE_TIMEOUT,
    });
    await expect(kanban.taskCardByTitle("Survivor Subtask A")).toBeVisible();
    await expect(kanban.taskCardByTitle("Survivor Subtask B")).toBeVisible();
  });

  test("ticking the cascade checkbox archives subtasks too", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const parent = await apiClient.createTask(seedData.workspaceId, "Parent Cascade Subs", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Doomed Subtask A", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });
    await apiClient.createTask(seedData.workspaceId, "Doomed Subtask B", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("Parent Cascade Subs")).toBeVisible({
      timeout: VISIBLE_TIMEOUT,
    });

    await kanban.openTaskActionsMenu(parent.id);
    await testPage.getByRole("menuitem", { name: "Archive" }).click();

    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    const cascade = testPage.getByTestId("archive-cascade-checkbox");
    await expect(cascade).toBeVisible();
    await cascade.click();
    await expect(cascade).toBeChecked();

    await dialog.getByRole("button", { name: "Archive" }).click();

    await expect(kanban.taskCardByTitle("Parent Cascade Subs")).not.toBeVisible({
      timeout: VISIBLE_TIMEOUT,
    });
    await expect(kanban.taskCardByTitle("Doomed Subtask A")).not.toBeVisible({
      timeout: VISIBLE_TIMEOUT,
    });
    await expect(kanban.taskCardByTitle("Doomed Subtask B")).not.toBeVisible({
      timeout: VISIBLE_TIMEOUT,
    });
  });
});

test.describe("Kanban card delete — cascade subtasks toggle", () => {
  test("deleting the parent without ticking the box reparents subtasks to root", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const parent = await apiClient.createTask(seedData.workspaceId, "Parent Delete Keep", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Orphaned Subtask", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("Parent Delete Keep")).toBeVisible({
      timeout: VISIBLE_TIMEOUT,
    });

    await kanban.openTaskActionsMenu(parent.id);
    await testPage.getByRole("menuitem", { name: "Delete" }).click();

    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    const cascade = testPage.getByTestId("delete-cascade-checkbox");
    await expect(cascade).toBeVisible();
    await expect(dialog).toContainText("Also delete 1 subtask");
    await expect(cascade).not.toBeChecked();

    await dialog.getByRole("button", { name: "Delete" }).click();

    // Parent is gone, child survives (now reparented to root with no
    // subtask badge — the badge requires a live parent on the board).
    await expect(kanban.taskCardByTitle("Parent Delete Keep")).not.toBeVisible({
      timeout: VISIBLE_TIMEOUT,
    });
    const survivor = kanban.taskCardByTitle("Orphaned Subtask");
    await expect(survivor).toBeVisible();
    await expect(survivor.getByText("Parent Delete Keep")).not.toBeVisible();
  });

  test("ticking the cascade checkbox deletes subtasks too", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const parent = await apiClient.createTask(seedData.workspaceId, "Parent Delete Cascade", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.createTask(seedData.workspaceId, "Cascaded Doomed Subtask", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      parent_id: parent.id,
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle("Parent Delete Cascade")).toBeVisible({
      timeout: VISIBLE_TIMEOUT,
    });

    await kanban.openTaskActionsMenu(parent.id);
    await testPage.getByRole("menuitem", { name: "Delete" }).click();

    const dialog = testPage.getByRole("alertdialog");
    const cascade = testPage.getByTestId("delete-cascade-checkbox");
    await expect(cascade).toBeVisible();
    await cascade.click();
    await expect(cascade).toBeChecked();

    await dialog.getByRole("button", { name: "Delete" }).click();

    await expect(kanban.taskCardByTitle("Parent Delete Cascade")).not.toBeVisible({
      timeout: VISIBLE_TIMEOUT,
    });
    await expect(kanban.taskCardByTitle("Cascaded Doomed Subtask")).not.toBeVisible({
      timeout: VISIBLE_TIMEOUT,
    });
  });
});
