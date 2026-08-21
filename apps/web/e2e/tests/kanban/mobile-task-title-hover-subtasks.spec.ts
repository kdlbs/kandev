import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("mobile task title hover on the Kanban card", () => {
  test("AC18/AC20: no hover card mounts on a coarse pointer, and tapping the title navigates as before", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      enable_preview_on_click: false,
    });
    // No agent, deliberately — see the desktop title-hover spec's rationale.
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile title hover parent", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const childTitle = "Mobile child title that stays available in the task switcher";
    await apiClient.createTask(seedData.workspaceId, childTitle, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      parent_id: task.id,
      workspace_mode: "inherit_parent",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCard(task.id);
    await expect(card).toBeVisible({ timeout: 45_000 });

    const title = card.getByTestId("task-card-title");
    await expect(testPage.getByTestId("task-title-hover-card")).toHaveCount(0);

    // Tapping the title performs the surface's existing navigation (opening
    // the task), not a hover disclosure — there is no separate touch target.
    await title.tap();
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));
    await expect(testPage.getByTestId("task-title-hover-card")).toHaveCount(0);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await testPage.getByTestId("mobile-session-menu").tap();
    const taskSwitcher = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(taskSwitcher.getByText(childTitle)).toBeVisible({ timeout: 15_000 });

    await assertNoDocumentHorizontalOverflow(testPage, "mobile Kanban board after tapping title");
  });
});
