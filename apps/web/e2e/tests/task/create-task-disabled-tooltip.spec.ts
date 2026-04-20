import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

const START_AGENT_TEST_ID = "submit-start-agent";
const WRAPPER_TEST_ID = "submit-start-agent-wrapper";
const START_ENABLED_TIMEOUT = 30_000;

test.describe("Create task button: disabled-reason tooltip", () => {
  test("shows 'Add a task title' when title is empty", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Leave title empty; fill description so the split button renders (showStartTask)
    await testPage.getByTestId("task-description-input").fill("some description");

    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeDisabled();

    // Hover the wrapper span (disabled button has pointer-events-none, tooltip
    // won't fire on the button itself — hover the span that wraps it).
    await testPage.getByTestId(WRAPPER_TEST_ID).hover();
    await expect(testPage.getByRole("tooltip")).toContainText("Add a task title", {
      timeout: 5_000,
    });
  });

  test("tooltip omits any disabled reason once the form is valid", async ({ testPage }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("Valid Task");
    await testPage.getByTestId("task-description-input").fill("doing a thing");

    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });

    // Hover still shows the keyboard shortcut tooltip, but none of the
    // disabled-reason strings should appear.
    await testPage.getByTestId(WRAPPER_TEST_ID).hover();
    const tooltip = testPage.getByRole("tooltip");
    await expect(tooltip).toBeVisible({ timeout: 5_000 });
    await expect(tooltip).not.toContainText("Add a task title");
    await expect(tooltip).not.toContainText("Select a repository");
    await expect(tooltip).not.toContainText("Select a branch");
    await expect(tooltip).not.toContainText("Select an agent");
  });

  test("shows 'Add a task title' after clearing a previously-filled title", async ({
    testPage,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    const titleInput = testPage.getByTestId("task-title-input");
    await titleInput.fill("Temp Title");
    await testPage.getByTestId("task-description-input").fill("description text");

    const startBtn = testPage.getByTestId(START_AGENT_TEST_ID);
    await expect(startBtn).toBeEnabled({ timeout: START_ENABLED_TIMEOUT });

    await titleInput.fill("");
    await expect(startBtn).toBeDisabled();

    await testPage.getByTestId(WRAPPER_TEST_ID).hover();
    await expect(testPage.getByRole("tooltip")).toContainText("Add a task title", {
      timeout: 5_000,
    });
  });
});
