import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";

useRegularMode();

test.describe("Task create relationships on mobile", () => {
  test("keeps Parent and Related-to selectors touch friendly and contained", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const parent = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile relationship parent",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    const related = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile relationship candidate",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.tap();
    const dialog = testPage.getByTestId("create-task-dialog");
    await dialog.getByTestId("task-create-advanced-settings-trigger").tap();
    const parentTrigger = dialog.getByTestId("task-create-parent-trigger");
    await expect
      .poll(async () => (await parentTrigger.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    await assertLocatorWithinViewportX(parentTrigger, "Parent selector");
    await parentTrigger.tap();
    await dialog.getByTestId(`task-create-parent-option-${parent.id}`).tap();
    const relatedTrigger = dialog.getByTestId("task-create-related-trigger");
    await expect
      .poll(async () => (await relatedTrigger.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    await assertLocatorWithinViewportX(relatedTrigger, "Related-to selector");
    await relatedTrigger.tap();
    await dialog.getByTestId(`task-create-related-option-${related.id}`).tap();
    await assertNoDocumentHorizontalOverflow(testPage, "relationship selector selections");

    await testPage.keyboard.press("Escape");
    await testPage.goto(`/t/${parent.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await testPage.getByTestId("mobile-task-picker-trigger").tap();
    const taskSheet = testPage.getByRole("dialog", { name: "Tasks" });
    const parentRow = taskSheet
      .getByTestId("sidebar-task-item")
      .filter({ hasText: "Mobile relationship parent" });
    await parentRow.getByRole("button", { name: "Task actions" }).tap();
    await testPage.getByRole("menuitem", { name: "Create Subtask", exact: true }).tap();
    const subtaskDialog = testPage.getByTestId("new-subtask-dialog");
    const subtaskRelated = subtaskDialog.getByTestId("task-create-related-trigger");
    await expect
      .poll(async () => (await subtaskRelated.boundingBox())?.height ?? 0)
      .toBeGreaterThanOrEqual(44);
    await subtaskRelated.tap();
    await subtaskDialog.getByTestId(`task-create-related-option-${related.id}`).tap();
    await assertLocatorWithinViewportX(subtaskRelated, "subtask Related-to selector");
    await assertNoDocumentHorizontalOverflow(testPage, "subtask relationship selector");
  });
});
