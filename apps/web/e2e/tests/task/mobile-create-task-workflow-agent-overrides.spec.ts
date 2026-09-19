import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { KanbanPage } from "../../pages/kanban-page";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import {
  createOverrideTask,
  deleteFixtureTasks,
  seedWorkflowAgentOverrideFixture,
  waitForNewWorkflowProfileSession,
  waitForWorkflowMoveLifecycle,
  waitForWorkflowStep,
} from "./task-workflow-agent-overrides-helpers";

useRegularMode();

test.describe("mobile: task-specific workflow agent overrides", () => {
  test("keeps touch controls usable and routes the selected profile", async ({
    testPage,
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const fixture = await seedWorkflowAgentOverrideFixture(apiClient, seedData, "Mobile Overrides");
    const createdTaskIds: string[] = [];
    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: fixture.workflow.id,
      task_create_last_used: {
        repository_id: seedData.repositoryId,
        branch: "main",
        agent_profile_id: fixture.profileA.id,
        workflow_ids_by_workspace: { [seedData.workspaceId]: fixture.workflow.id },
      },
      enable_preview_on_click: true,
    });

    try {
      const mobile = new MobileKanbanPage(testPage);
      await mobile.goto();
      await mobile.mobileFab.tap();

      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await dialog.getByTestId("task-title-input").fill("Mobile workflow override task");
      await dialog.getByTestId("task-description-input").fill("Mobile override description");
      await dialog.getByTestId("task-create-advanced-settings-trigger").tap();

      const row = dialog
        .getByTestId("task-create-workflow-agent-overrides")
        .getByTestId(`task-create-workflow-agent-override-${fixture.profileA.id}`);
      await expect(row).toBeVisible({ timeout: 20_000 });
      await expect(row).toContainText("Implement");
      await expect(row).toContainText("PR");
      const selector = row.getByTestId("agent-profile-selector");
      const selectorBox = await selector.boundingBox();
      expect(selectorBox).not.toBeNull();
      if (!selectorBox) throw new Error("mobile override selector has no layout box");
      expect(selectorBox.height).toBeGreaterThanOrEqual(44);

      await selector.tap();
      const replacementOption = testPage.getByRole("option", {
        name: new RegExp(fixture.profileB.name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")),
      });
      await expect(replacementOption).toBeVisible();
      await replacementOption.tap();
      await expect(selector).toContainText(fixture.profileB.name);

      await dialog.getByTestId("task-create-workflow-agent-overrides-reset").tap();
      await expect(selector).toContainText("Use workflow profile");
      await selector.tap();
      await testPage
        .getByRole("option", {
          name: new RegExp(fixture.profileB.name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")),
        })
        .tap();
      await expect(selector).toContainText(fixture.profileB.name);
      await assertNoDocumentHorizontalOverflow(testPage);

      await dialog.getByTestId("mobile-create-without-agent").tap();
      await expect(dialog).not.toBeVisible({ timeout: 15_000 });
      let createdTaskId = "";
      await expect
        .poll(async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          createdTaskId =
            tasks.find((task) => task.title === "Mobile workflow override task")?.id ?? "";
          return createdTaskId;
        })
        .not.toBe("");
      createdTaskIds.push(createdTaskId);
      expect((await apiClient.getTask(createdTaskId)).workflow_agent_overrides?.steps).toEqual([
        expect.objectContaining({
          step_id: fixture.implementStep.id,
          replacement_profile_id: fixture.profileB.id,
        }),
      ]);

      const runtimeTask = await createOverrideTask(apiClient, seedData, fixture, {
        title: "Mobile runtime override",
        replacementProfileId: fixture.profileB.id,
      });
      createdTaskIds.push(runtimeTask.id);
      const initialSessionId = await waitForNewWorkflowProfileSession(
        apiClient,
        runtimeTask.id,
        fixture.profileA.id,
      );

      const tablet = new KanbanPage(tabletTestPage);
      await tablet.goto();
      const card = tablet.taskCard(runtimeTask.id);
      await expect(card).toBeVisible({ timeout: 15_000 });
      await card.tap();
      const previewPanel = tabletTestPage.getByTestId("task-preview-panel");
      await expect(previewPanel).toBeVisible({ timeout: 15_000 });
      const trigger = previewPanel.getByTestId("workflow-stepper-minimal");
      await trigger.tap();
      const drawer = tabletTestPage.locator('[data-slot="drawer-content"][data-state="open"]');
      await expect(drawer).toBeVisible();
      const targetRow = drawer.getByTestId(
        `workflow-step-disclosure-row-${fixture.implementStep.id}`,
      );
      await expect(targetRow).toBeVisible();
      await expect(targetRow.getByTestId("workflow-move-preview")).toContainText("mock-slow");
      const move = targetRow.getByTestId(
        `workflow-step-disclosure-move-${fixture.implementStep.id}`,
      );
      const moveBox = await move.boundingBox();
      expect(moveBox).not.toBeNull();
      if (!moveBox) throw new Error("mobile workflow move control has no layout box");
      expect(moveBox.height).toBeGreaterThanOrEqual(44);
      await targetRow
        .getByTestId(`workflow-step-disclosure-move-${fixture.implementStep.id}`)
        .tap();

      await waitForWorkflowStep(apiClient, runtimeTask.id, fixture.implementStep.id);
      const replacementSessionId = await waitForNewWorkflowProfileSession(
        apiClient,
        runtimeTask.id,
        fixture.profileB.id,
        [initialSessionId],
      );
      await expect
        .poll(() => apiClient.getTask(runtimeTask.id).then((task) => task.primary_session_id))
        .toBe(replacementSessionId);
      await waitForWorkflowMoveLifecycle(apiClient, runtimeTask.id);
      await assertNoDocumentHorizontalOverflow(tabletTestPage);
    } finally {
      await deleteFixtureTasks(apiClient, createdTaskIds);
      await apiClient.deleteWorkflow(fixture.workflow.id).catch(() => undefined);
    }
  });
});
