import { test, expect } from "../../fixtures/test-base";
import { assertNoDescendantOverflowsRight } from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

// Exercises the regular task/subtask dialogs, so run with the office feature disabled.
useRegularMode();

const LONG_UNBROKEN_TEXT = `review-${"x".repeat(240)}-https://github.com/example/repo/commit/${"a".repeat(80)}`;

test.describe("Dialog long text layout", () => {
  test("agent, task, and subtask dialogs stay within their modal bounds", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const createDialog = testPage.getByTestId("create-task-dialog");
    await expect(createDialog).toBeVisible();
    await testPage.getByTestId("task-title-input").fill(LONG_UNBROKEN_TEXT);
    await testPage.getByTestId("task-description-input").fill(LONG_UNBROKEN_TEXT);
    await assertNoDescendantOverflowsRight(createDialog, "create task dialog");
    await testPage.keyboard.press("Escape");
    await expect(createDialog).not.toBeVisible();

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Long Text Dialog Parent",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await session.openNewSessionDialog();
    const newSessionDialog = session.newSessionDialog();
    await expect(newSessionDialog).toBeVisible();
    await session.newSessionPromptInput().fill(LONG_UNBROKEN_TEXT);
    await assertNoDescendantOverflowsRight(newSessionDialog, "new agent dialog");
    await testPage.keyboard.press("Escape");
    await expect(newSessionDialog).not.toBeVisible();

    await testPage.getByTestId("sidebar-new-subtask").click();
    const subtaskDialog = testPage.getByTestId("new-subtask-dialog");
    await expect(subtaskDialog).toBeVisible();
    await testPage.getByTestId("subtask-title-input").fill(LONG_UNBROKEN_TEXT);
    await testPage.getByTestId("subtask-prompt-input").fill(LONG_UNBROKEN_TEXT);
    await assertNoDescendantOverflowsRight(subtaskDialog, "new subtask dialog");
  });
});
