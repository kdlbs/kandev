import { expect, test } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import { useRegularMode } from "../../helpers/regular-mode";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { dwell } from "../../helpers/causal-waits";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";
import { waitForSessionDone } from "../../helpers/session";
import {
  cleanupColdWorkflowMetadataScenario,
  createOverrideTask,
  deleteFixtureTasks,
  hasWorkflowSnapshot,
  recordWorkflowMetadataRequests,
  saveColdWorkflowMetadataSettings,
  seedColdWorkflowMetadataScenario,
  seedWorkflowAgentOverrideFixture,
  previewWorkflowMove,
  waitForLoadedBoardSnapshot,
  waitForNewWorkflowProfileSession,
  waitForWorkflowMoveLifecycle,
  waitForWorkflowStep,
  workflowMetadataRequests,
} from "./task-workflow-agent-overrides-helpers";

test.use({ trace: "retain-on-failure" });

useRegularMode();

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

async function selectColdWorkflow(
  page: Page,
  dialog: Locator,
  workflowName: string,
): Promise<void> {
  await dialog.getByTestId("workflow-selector-trigger").tap();
  await page
    .getByRole("button", { name: new RegExp(`^${escapeRegExp(workflowName)}`) })
    .last()
    .tap();
  await expect(page.getByTestId("workflow-selector-popover")).toHaveCount(0);
}

async function waitForCreatedTask(
  apiClient: Parameters<typeof deleteFixtureTasks>[0],
  workspaceId: string,
  title: string,
): Promise<string> {
  let createdTaskId = "";
  await expect
    .poll(async () => {
      const { tasks } = await apiClient.listTasks(workspaceId);
      createdTaskId = tasks.find((task) => task.title === title)?.id ?? "";
      return createdTaskId;
    })
    .not.toBe("");
  return createdTaskId;
}

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
    await apiClient.updateWorkflowStep(fixture.implementStep.id, {
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
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
      // Finish the initial launch before changing the bound session's model.
      await expect
        .poll(async () => {
          const { session } = await apiClient.getTaskSession(replacementSessionId);
          const runtime = session.metadata?.runtime_config as { model?: string } | undefined;
          return { state: session.state, model: runtime?.model };
        })
        .toEqual({ state: "WAITING_FOR_INPUT", model: "mock-slow" });
      // Reuse must follow the bound session and its current model, not its profile default.
      await apiClient.setSessionModel(replacementSessionId, "mock-smart");
      await apiClient.moveTask(runtimeTask.id, fixture.workflow.id, fixture.reviewStep.id);
      await waitForWorkflowStep(apiClient, runtimeTask.id, fixture.reviewStep.id);
      await expect
        .poll(() => apiClient.getTask(runtimeTask.id).then((task) => task.primary_session_id))
        .toBe(initialSessionId);
      await waitForWorkflowMoveLifecycle(apiClient, runtimeTask.id);
      await waitForSessionDone(
        apiClient,
        runtimeTask.id,
        initialSessionId,
        "The resumed initial session must be idle before workflow progression is available",
        45_000,
      );

      const actualPreview = await previewWorkflowMove(
        apiClient,
        runtimeTask.id,
        fixture.workflow.id,
        fixture.prStep.id,
      );
      expect(actualPreview.recipient).toEqual(
        expect.objectContaining({
          session_id: replacementSessionId,
          profile_id: fixture.profileB.id,
        }),
      );
      expect(actualPreview.model.after).toEqual(
        expect.objectContaining({ known: true, id: "mock-smart" }),
      );

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
      await expect(actualTopbarRow.getByTestId("workflow-move-preview")).toContainText(
        "mock-smart",
      );
      await tabletTestPage.keyboard.press("Escape");

      await testPage.reload();
      await phoneSession.waitForLoad();
      // Reload can restore the parked runtime after the earlier API idle check.
      await phoneSession.waitForChatIdle({ timeout: 45_000, requireEditable: true });
      const actualAboveChatButton = testPage.getByTestId("proceed-next-step");
      await expect(actualAboveChatButton).toBeVisible();
      await longPress(testPage, actualAboveChatButton);
      const actualAboveChatOptions = testPage.getByTestId("workflow-move-options");
      await expect(actualAboveChatOptions).toBeVisible();
      await expect(actualAboveChatOptions.getByTestId("workflow-move-preview")).toContainText(
        "mock-smart",
      );
      await actualAboveChatOptions.getByRole("button", { name: "Cancel", exact: true }).click();
      await assertNoDocumentHorizontalOverflow(tabletTestPage);
      await assertNoDocumentHorizontalOverflow(testPage);
    } finally {
      await deleteFixtureTasks(apiClient, createdTaskIds);
      await apiClient.deleteWorkflow(fixture.workflow.id).catch(() => undefined);
    }
  });

  test("cold workflow metadata loads an explicitly selected unopened workflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const scenario = await seedColdWorkflowMetadataScenario(
      apiClient,
      seedData,
      "Cold Explicit Mobile",
    );
    const { fixture, boardWorkflow } = scenario;
    await saveColdWorkflowMetadataSettings(apiClient, seedData, scenario, false);
    const requests = recordWorkflowMetadataRequests(testPage);
    const createdTaskIds: string[] = [];

    try {
      const mobile = new MobileKanbanPage(testPage);
      await mobile.goto();
      await waitForLoadedBoardSnapshot(testPage, boardWorkflow.id);
      expect(await hasWorkflowSnapshot(testPage, fixture.workflow.id)).toBe(false);

      await mobile.mobileFab.tap();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("workflow-selector-trigger")).toContainText(
        boardWorkflow.name,
      );
      await selectColdWorkflow(testPage, dialog, fixture.workflow.name);
      await dialog.getByTestId("task-title-input").fill("Cold explicit mobile task");
      await dialog.getByTestId("task-description-input").fill("Cold explicit mobile description");
      await dialog.getByTestId("task-create-advanced-settings-trigger").tap();

      const overrides = dialog.getByTestId("task-create-workflow-agent-overrides");
      await expect(overrides).toBeVisible({ timeout: 20_000 });
      const row = overrides.getByTestId(
        `task-create-workflow-agent-override-${fixture.profileA.id}`,
      );
      await expect(row).toContainText("Implement");
      await expect(row).toContainText("PR");
      await expect(dialog.getByTestId("task-create-launch-preview-toggle")).toHaveAttribute(
        "aria-label",
        /PR/,
      );
      const selector = row.getByTestId("agent-profile-selector");
      const selectorBox = await selector.boundingBox();
      expect(selectorBox).not.toBeNull();
      if (!selectorBox) throw new Error("mobile cold override selector has no layout box");
      expect(selectorBox.height).toBeGreaterThanOrEqual(44);

      await selector.tap();
      await testPage
        .getByRole("option", { name: new RegExp(escapeRegExp(fixture.profileB.name)) })
        .tap();
      await expect(selector).toContainText(fixture.profileB.name);
      await overrides.getByTestId("task-create-workflow-agent-overrides-reset").tap();
      await expect(selector).toContainText("Use workflow profile");
      await selector.tap();
      await testPage
        .getByRole("option", { name: new RegExp(escapeRegExp(fixture.profileB.name)) })
        .tap();
      await assertNoDocumentHorizontalOverflow(testPage);

      await dialog.getByTestId("mobile-create-without-agent").tap();
      await expect(dialog).not.toBeVisible({ timeout: 15_000 });
      const createdTaskId = await waitForCreatedTask(
        apiClient,
        seedData.workspaceId,
        "Cold explicit mobile task",
      );
      createdTaskIds.push(createdTaskId);
      expect((await apiClient.getTask(createdTaskId)).workflow_agent_overrides?.steps).toEqual([
        expect.objectContaining({
          step_id: fixture.implementStep.id,
          replacement_profile_id: fixture.profileB.id,
        }),
      ]);
      expect(workflowMetadataRequests(requests, fixture.workflow.id, "snapshot")).toHaveLength(0);
      expect(
        workflowMetadataRequests(requests, fixture.workflow.id, "steps").length,
      ).toBeGreaterThan(0);
    } finally {
      await cleanupColdWorkflowMetadataScenario(apiClient, scenario, createdTaskIds);
    }
  });

  test("cold workflow metadata recovers a remembered unopened workflow and keeps the draft", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const scenario = await seedColdWorkflowMetadataScenario(
      apiClient,
      seedData,
      "Cold Remembered Mobile",
    );
    const { fixture, boardWorkflow } = scenario;
    await saveColdWorkflowMetadataSettings(apiClient, seedData, scenario, true);
    const stepRoute = `**/api/v1/workflows/${fixture.workflow.id}/workflow/steps`;
    let failedOnce = false;
    await testPage.route(stepRoute, async (route) => {
      if (!failedOnce) {
        failedOnce = true;
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "temporarily unavailable" }),
        });
        return;
      }
      await route.continue();
    });
    const requests = recordWorkflowMetadataRequests(testPage);
    const createdTaskIds: string[] = [];

    try {
      const mobile = new MobileKanbanPage(testPage);
      await mobile.goto();
      await waitForLoadedBoardSnapshot(testPage, boardWorkflow.id);
      expect(await hasWorkflowSnapshot(testPage, fixture.workflow.id)).toBe(false);

      await mobile.mobileFab.tap();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("workflow-selector-trigger")).toContainText(
        fixture.workflow.name,
      );
      await dialog.getByTestId("task-title-input").fill("Cold remembered mobile task");
      await dialog.getByTestId("task-description-input").fill("Cold remembered mobile description");
      await dialog.getByTestId("task-create-advanced-settings-trigger").tap();

      const overrides = dialog.getByTestId("task-create-workflow-agent-overrides");
      const retry = overrides.getByTestId("task-create-workflow-agent-overrides-retry");
      await expect(retry).toBeVisible({ timeout: 20_000 });
      const retryBox = await retry.boundingBox();
      expect(retryBox).not.toBeNull();
      if (!retryBox) throw new Error("mobile cold retry has no layout box");
      expect(retryBox.height).toBeGreaterThanOrEqual(44);
      await expect(dialog.getByTestId("task-title-input")).toHaveValue(
        "Cold remembered mobile task",
      );

      await retry.tap();
      const row = overrides.getByTestId(
        `task-create-workflow-agent-override-${fixture.profileA.id}`,
      );
      await expect(row).toBeVisible({ timeout: 20_000 });
      await expect(row).toContainText("Implement");
      await expect(row).toContainText("PR");
      const selector = row.getByTestId("agent-profile-selector");
      const selectorBox = await selector.boundingBox();
      expect(selectorBox).not.toBeNull();
      if (!selectorBox) throw new Error("mobile cold remembered selector has no layout box");
      expect(selectorBox.height).toBeGreaterThanOrEqual(44);
      await selector.tap();
      await testPage
        .getByRole("option", { name: new RegExp(escapeRegExp(fixture.profileB.name)) })
        .tap();
      await assertNoDocumentHorizontalOverflow(testPage);

      await dialog.getByTestId("mobile-create-without-agent").tap();
      await expect(dialog).not.toBeVisible({ timeout: 15_000 });
      const createdTaskId = await waitForCreatedTask(
        apiClient,
        seedData.workspaceId,
        "Cold remembered mobile task",
      );
      createdTaskIds.push(createdTaskId);
      const stored = await apiClient.getTask(createdTaskId);
      expect(stored.title).toBe("Cold remembered mobile task");
      expect(stored.description).toContain("Cold remembered mobile description");
      expect(stored.workflow_agent_overrides?.steps).toEqual([
        expect.objectContaining({
          step_id: fixture.implementStep.id,
          replacement_profile_id: fixture.profileB.id,
        }),
      ]);
      expect(workflowMetadataRequests(requests, fixture.workflow.id, "snapshot")).toHaveLength(0);
      expect(
        workflowMetadataRequests(requests, fixture.workflow.id, "steps").length,
      ).toBeGreaterThanOrEqual(2);
    } finally {
      await testPage.unroute(stepRoute).catch(() => undefined);
      await cleanupColdWorkflowMetadataScenario(apiClient, scenario, createdTaskIds);
    }
  });
});
