import { test, expect } from "../../fixtures/test-base";
import type { Locator } from "@playwright/test";
import {
  assertLocatorWithinViewportX,
  assertNoDescendantOverflowsRight,
} from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

// Exercises the regular task/subtask dialogs, so run with the office feature disabled.
useRegularMode();

const LONG_UNBROKEN_TEXT = `review-${"x".repeat(240)}-https://github.com/example/repo/commit/${"a".repeat(80)}`;

async function assertProfileDropdownFits(trigger: Locator, dropdownLabel: string) {
  await trigger.click();
  const dropdown = trigger
    .page()
    .locator('[data-slot="popover-content"]')
    .filter({ hasText: dropdownLabel })
    .last();
  await expect(dropdown).toBeVisible();
  await assertLocatorWithinViewportX(dropdown, dropdownLabel);
  const searchInput = dropdown.getByPlaceholder(/Search (agents|profiles)/);
  if ((await searchInput.count()) > 0) {
    await searchInput.click();
    await expect(searchInput).toBeFocused();
  }
  const option = dropdown.getByRole("option").first();
  await expect(option).toBeVisible();
  await option.click();
  await expect(dropdown).not.toBeVisible();
}

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
    await assertProfileDropdownFits(
      testPage.getByTestId("agent-profile-selector"),
      "Agent Profile",
    );
    await assertProfileDropdownFits(
      testPage.getByTestId("executor-profile-selector"),
      "Executor Profile",
    );
    await createDialog.getByRole("button", { name: "Cancel" }).click();
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
    if ((await testPage.getByTestId("agent-profile-selector").count()) > 0) {
      await assertProfileDropdownFits(
        testPage.getByTestId("agent-profile-selector"),
        "Agent Profile",
      );
    }
    await newSessionDialog.getByRole("button", { name: "Cancel" }).click();
    await expect(newSessionDialog).not.toBeVisible();

    await testPage.getByTestId("sidebar-new-subtask").click();
    const subtaskDialog = testPage.getByTestId("new-subtask-dialog");
    await expect(subtaskDialog).toBeVisible();
    await testPage.getByTestId("subtask-title-input").fill(LONG_UNBROKEN_TEXT);
    await testPage.getByTestId("subtask-prompt-input").fill(LONG_UNBROKEN_TEXT);
    await assertNoDescendantOverflowsRight(subtaskDialog, "new subtask dialog");
    await assertProfileDropdownFits(
      testPage.getByTestId("agent-profile-selector"),
      "Agent Profile",
    );
    if ((await testPage.getByTestId("executor-profile-selector").count()) > 0) {
      await assertProfileDropdownFits(
        testPage.getByTestId("executor-profile-selector"),
        "Executor Profile",
      );
    }
  });
});
