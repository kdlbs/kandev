import { expect, test } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import { useRegularMode } from "../../helpers/regular-mode";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { dwell } from "../../helpers/causal-waits";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";
import {
  createOverrideTask,
  deleteFixtureTasks,
  seedWorkflowAgentOverrideFixture,
  waitForNewWorkflowProfileSession,
  waitForWorkflowMoveLifecycle,
  waitForWorkflowStep,
} from "./task-workflow-agent-overrides-helpers";

useRegularMode();

async function longPress(page: Page, target: Locator): Promise<void> {
  await target.scrollIntoViewIfNeeded();
  const bounds = await target.boundingBox();
  if (!bounds) throw new Error("next-step button has no bounding box");

  const cdp = await page.context().newCDPSession(page);
  const x = bounds.x + bounds.width / 2;
  const y = bounds.y + bounds.height / 2;
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x, y }],
  });
  await dwell(page, 600, "library-timer", "workflow move long-press threshold settles");
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
}

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

      const tabletSession = new SessionPage(tabletTestPage);
      await tabletTestPage.goto(`/t/${runtimeTask.id}`);
      await tabletSession.waitForLoad();
      const tabletStepper = tabletTestPage.getByTestId("workflow-stepper-minimal");
      await expect(tabletStepper).toBeVisible();
      await tabletStepper.tap();
      const tabletDrawer = tabletTestPage.locator(
        '[data-slot="drawer-content"][data-state="open"]',
      );
      await expect(tabletDrawer).toBeVisible();
      const plannedTopbarRow = tabletDrawer.getByTestId(
        `workflow-step-disclosure-row-${fixture.implementStep.id}`,
      );
      await expect(plannedTopbarRow.getByTestId("workflow-move-preview")).toContainText(
        "mock-slow",
      );
      const topbarMove = plannedTopbarRow.getByTestId(
        `workflow-step-disclosure-move-${fixture.implementStep.id}`,
      );
      const topbarMoveBox = await topbarMove.boundingBox();
      expect(topbarMoveBox).not.toBeNull();
      if (!topbarMoveBox) throw new Error("mobile workflow move control has no layout box");
      expect(topbarMoveBox.height).toBeGreaterThanOrEqual(44);
      await tabletTestPage.keyboard.press("Escape");

      const phoneSession = new SessionPage(testPage);
      await testPage.goto(`/t/${runtimeTask.id}`);
      await phoneSession.waitForLoad();
      const plannedAboveChatButton = testPage.getByTestId("proceed-next-step");
      await expect(plannedAboveChatButton).toBeVisible();
      await longPress(testPage, plannedAboveChatButton);
      const plannedAboveChatOptions = testPage.getByTestId("workflow-move-options");
      await expect(plannedAboveChatOptions).toBeVisible();
      await expect(plannedAboveChatOptions.getByTestId("workflow-move-preview")).toContainText(
        "mock-slow",
      );
      await plannedAboveChatOptions.getByRole("button", { name: "Cancel", exact: true }).click();

      await apiClient.moveTask(runtimeTask.id, fixture.workflow.id, fixture.implementStep.id);
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
      await apiClient.moveTask(runtimeTask.id, fixture.workflow.id, fixture.reviewStep.id);
      await waitForWorkflowStep(apiClient, runtimeTask.id, fixture.reviewStep.id);
      await expect
        .poll(() => apiClient.getTask(runtimeTask.id).then((task) => task.primary_session_id))
        .toBe(initialSessionId);
      await waitForWorkflowMoveLifecycle(apiClient, runtimeTask.id);

      await tabletTestPage.reload();
      await tabletSession.waitForLoad();
      await tabletStepper.tap();
      const actualTabletDrawer = tabletTestPage.locator(
        '[data-slot="drawer-content"][data-state="open"]',
      );
      await expect(actualTabletDrawer).toBeVisible();
      const actualTopbarRow = actualTabletDrawer.getByTestId(
        `workflow-step-disclosure-row-${fixture.prStep.id}`,
      );
      await expect(actualTopbarRow.getByTestId("workflow-move-preview")).toContainText("mock-slow");
      await tabletTestPage.keyboard.press("Escape");

      await testPage.reload();
      await phoneSession.waitForLoad();
      const actualAboveChatButton = testPage.getByTestId("proceed-next-step");
      await expect(actualAboveChatButton).toBeVisible();
      await longPress(testPage, actualAboveChatButton);
      const actualAboveChatOptions = testPage.getByTestId("workflow-move-options");
      await expect(actualAboveChatOptions).toBeVisible();
      await expect(actualAboveChatOptions.getByTestId("workflow-move-preview")).toContainText(
        "mock-slow",
      );
      await actualAboveChatOptions.getByRole("button", { name: "Cancel", exact: true }).click();
      await assertNoDocumentHorizontalOverflow(tabletTestPage);
      await assertNoDocumentHorizontalOverflow(testPage);
    } finally {
      await deleteFixtureTasks(apiClient, createdTaskIds);
      await apiClient.deleteWorkflow(fixture.workflow.id).catch(() => undefined);
    }
  });
});
