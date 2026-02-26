import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";

test.describe("Task creation", () => {
  test("opens create task dialog from kanban header", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    await expect(testPage.getByTestId("create-task-dialog")).toBeVisible();
  });

  test("can fill in task title and description", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    const titleInput = testPage.getByTestId("task-title-input");
    await titleInput.fill("My E2E Test Task");
    await expect(titleInput).toHaveValue("My E2E Test Task");

    const descInput = testPage.getByTestId("task-description-input");
    await descInput.fill("This is a test description");
    await expect(descInput).toHaveValue("This is a test description");
  });
});
