import { expect, test } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import { useRegularMode } from "../../helpers/regular-mode";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { waitForSessionDone } from "../../helpers/session";
import {
  cleanupColdWorkflowMetadataScenario,
  createOverrideTask,
  deleteFixtureTasks,
  hasWorkflowSnapshot,
  previewWorkflowMove,
  recordWorkflowMetadataRequests,
  saveColdWorkflowMetadataSettings,
  seedColdWorkflowMetadataScenario,
  seedWorkflowAgentOverrideFixture,
  waitForLoadedBoardSnapshot,
  waitForNewWorkflowProfileSession,
  waitForWorkflowMoveLifecycle,
  waitForWorkflowStep,
  workflowMetadataRequests,
} from "./task-workflow-agent-overrides-helpers";

useRegularMode();

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function taskIdFromUrl(url: string): string {
  const match = url.match(/\/t\/([^/?#]+)/);
  if (!match) throw new Error(`task id missing from URL: ${url}`);
  return match[1];
}

async function selectWorkflow(page: Page, dialog: Locator, workflowName: string) {
  await dialog.getByTestId("workflow-selector-trigger").click();
  await page
    .getByRole("button", { name: new RegExp(`^${escapeRegExp(workflowName)}`) })
    .last()
    .click();
  await expect(page.getByTestId("workflow-selector-popover")).toHaveCount(0);
}

test.describe("task-specific workflow agent overrides", () => {
  test("keeps grouped replacements task-local and exposes the planned model on desktop", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    await testPage.setViewportSize({ width: 1440, height: 900 });
    const fixture = await seedWorkflowAgentOverrideFixture(
      apiClient,
      seedData,
      "Desktop Overrides",
    );
    const otherWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Desktop Overrides Other Workflow",
    );
    await apiClient.createWorkflowStep(otherWorkflow.id, "Other start", 0, {
      is_start_step: true,
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
    });

    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await kanban.createTaskButton.first().click();

      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      const workflowSelector = dialog.getByTestId("workflow-selector-trigger");
      await expect(workflowSelector).toContainText(fixture.workflow.name);

      const advanced = dialog.getByTestId("task-create-advanced-settings");
      await advanced.getByTestId("task-create-advanced-settings-trigger").click();
      const overrides = dialog.getByTestId("task-create-workflow-agent-overrides");
      await expect(overrides).toBeVisible({ timeout: 20_000 });
      const sourceRow = overrides.getByTestId(
        `task-create-workflow-agent-override-${fixture.profileA.id}`,
      );
      await expect(sourceRow).toBeVisible();
      await expect(sourceRow).toContainText("Implement");
      await expect(sourceRow).toContainText("PR");

      const replacementSelector = sourceRow.getByTestId("agent-profile-selector");
      await replacementSelector.click();
      await expect(
        testPage.getByRole("option", { name: new RegExp(escapeRegExp(fixture.profileB.name)) }),
      ).toBeVisible();
      await testPage
        .getByRole("option", { name: new RegExp(escapeRegExp(fixture.profileB.name)) })
        .click();
      await expect(replacementSelector).toContainText(fixture.profileB.name);

      await advanced.getByTestId("task-create-advanced-settings-trigger").click();
      await expect(overrides).toBeHidden();
      await advanced.getByTestId("task-create-advanced-settings-trigger").click();
      await expect(replacementSelector).toContainText(fixture.profileB.name);

      await selectWorkflow(testPage, dialog, otherWorkflow.name);
      await expect(
        dialog.getByTestId(`task-create-workflow-agent-override-${fixture.profileA.id}`),
      ).toHaveCount(0);
      await selectWorkflow(testPage, dialog, fixture.workflow.name);
      const restoredRow = dialog.getByTestId(
        `task-create-workflow-agent-override-${fixture.profileA.id}`,
      );
      await expect(restoredRow.getByTestId("agent-profile-selector")).toContainText(
        "Use workflow profile",
      );
      await restoredRow.getByTestId("agent-profile-selector").click();
      await testPage
        .getByRole("option", { name: new RegExp(escapeRegExp(fixture.profileB.name)) })
        .click();

      await dialog.getByTestId("task-title-input").fill("Desktop workflow override task");
      await dialog.getByTestId("task-description-input").fill("Desktop override description");
      await dialog.getByTestId("submit-start-agent-chevron").click();
      await testPage.getByTestId("submit-create-without-agent").click();
      await expect(dialog).not.toBeVisible({ timeout: 15_000 });

      const createdTaskId = taskIdFromUrl(testPage.url());
      createdTaskIds.push(createdTaskId);
      const stored = await apiClient.getTask(createdTaskId);
      expect(stored.workflow_agent_overrides).toMatchObject({
        workflow_id: fixture.workflow.id,
        steps: [
          {
            step_id: fixture.implementStep.id,
            source_profile_id: fixture.profileA.id,
            replacement_profile_id: fixture.profileB.id,
          },
        ],
      });
      await testPage.reload();
      expect((await apiClient.getTask(createdTaskId)).workflow_agent_overrides).toEqual(
        stored.workflow_agent_overrides,
      );

      await kanban.goto();
      await kanban.createTaskButton.first().click();
      const freshDialog = testPage.getByTestId("create-task-dialog");
      await expect(freshDialog).toBeVisible();
      await freshDialog.getByTestId("task-create-advanced-settings-trigger").click();
      const freshRow = freshDialog
        .getByTestId("task-create-workflow-agent-overrides")
        .getByTestId(`task-create-workflow-agent-override-${fixture.profileA.id}`);
      await expect(freshRow.getByTestId("agent-profile-selector")).toContainText(
        "Use workflow profile",
      );
      await freshDialog.getByRole("button", { name: "Cancel", exact: true }).click();

      const runtimeTask = await createOverrideTask(apiClient, seedData, fixture, {
        title: "Desktop runtime override",
        replacementProfileId: fixture.profileB.id,
      });
      createdTaskIds.push(runtimeTask.id);
      const initialSessionId = await waitForNewWorkflowProfileSession(
        apiClient,
        runtimeTask.id,
        fixture.profileA.id,
      );
      const sessionPage = new SessionPage(testPage);

      await testPage.goto(`/t/${runtimeTask.id}`);
      await sessionPage.waitForLoad();
      const plannedTopbarStep = testPage.getByTestId("workflow-step-Implement");
      await expect(plannedTopbarStep).toBeVisible();
      await plannedTopbarStep.hover();
      const plannedTopbarPopover = testPage.getByTestId("workflow-step-popover");
      await expect(plannedTopbarPopover).toBeVisible();
      await expect(plannedTopbarPopover.getByTestId("workflow-move-preview")).toContainText(
        "mock-slow",
      );
      await testPage.mouse.move(0, 0);

      const plannedAboveChatButton = sessionPage.activeChat().getByTestId("proceed-next-step");
      await expect(plannedAboveChatButton).toBeVisible();
      await plannedAboveChatButton.hover();
      const plannedAboveChatOptions = testPage.getByTestId("proceed-next-step-options");
      await expect(plannedAboveChatOptions).toBeVisible();
      await expect(plannedAboveChatOptions.getByTestId("workflow-move-preview")).toContainText(
        "mock-slow",
      );
      await testPage.mouse.move(0, 0);
      await plannedAboveChatButton.click();
      await waitForWorkflowStep(apiClient, runtimeTask.id, fixture.implementStep.id);
      await waitForNewWorkflowProfileSession(apiClient, runtimeTask.id, fixture.profileB.id, [
        initialSessionId,
      ]);
      await waitForWorkflowMoveLifecycle(apiClient, runtimeTask.id);

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
        "Reused workflow session settled before proceeding",
      );
      await testPage.reload();
      await sessionPage.waitForLoad();

      const actualTopbarStep = testPage.getByTestId("workflow-step-PR");
      await expect(actualTopbarStep).toBeVisible();
      await actualTopbarStep.hover();
      const actualTopbarPopover = testPage.getByTestId("workflow-step-popover");
      await expect(actualTopbarPopover).toBeVisible();
      await expect(actualTopbarPopover.getByTestId("workflow-move-preview")).toContainText(
        "mock-slow",
      );
      await testPage.mouse.move(0, 0);

      const actualAboveChatButton = sessionPage.activeChat().getByTestId("proceed-next-step");
      await expect(actualAboveChatButton).toBeVisible();
      await actualAboveChatButton.hover();
      const actualAboveChatOptions = testPage.getByTestId("proceed-next-step-options");
      await expect(actualAboveChatOptions).toBeVisible();
      await expect(actualAboveChatOptions.getByTestId("workflow-move-preview")).toContainText(
        "mock-slow",
      );
      await testPage.mouse.move(0, 0);

      const planned = await previewWorkflowMove(
        apiClient,
        createdTaskId,
        fixture.workflow.id,
        fixture.prStep.id,
      );
      expect(planned.model.after.known).toBe(true);
      expect(planned.model.after.label).toContain("mock-slow");
      expect(planned.recipient?.profile_id).toBe(fixture.profileB.id);
    } finally {
      await deleteFixtureTasks(apiClient, createdTaskIds);
      await apiClient.deleteWorkflow(otherWorkflow.id).catch(() => undefined);
    }
  });

  test("routes three interleaved tasks independently through reused workflow sessions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const fixture = await seedWorkflowAgentOverrideFixture(
      apiClient,
      seedData,
      "Runtime Overrides",
    );
    const [defaultTask, replacementBTask, replacementCTask] = await Promise.all([
      createOverrideTask(apiClient, seedData, fixture, { title: "Runtime default" }),
      createOverrideTask(apiClient, seedData, fixture, {
        title: "Runtime replacement B",
        replacementProfileId: fixture.profileB.id,
      }),
      createOverrideTask(apiClient, seedData, fixture, {
        title: "Runtime replacement C",
        replacementProfileId: fixture.profileC.id,
      }),
    ]);
    const tasks = [defaultTask, replacementBTask, replacementCTask];
    const initialSessionIds = await Promise.all(
      tasks.map((task) =>
        waitForNewWorkflowProfileSession(apiClient, task.id, fixture.profileA.id),
      ),
    );

    try {
      expect((await apiClient.getTask(defaultTask.id)).workflow_agent_overrides).toBeUndefined();
      expect(
        (await apiClient.getTask(replacementBTask.id)).workflow_agent_overrides?.steps,
      ).toEqual([
        expect.objectContaining({
          step_id: fixture.implementStep.id,
          replacement_profile_id: fixture.profileB.id,
        }),
      ]);
      expect(
        (await apiClient.getTask(replacementCTask.id)).workflow_agent_overrides?.steps,
      ).toEqual([
        expect.objectContaining({
          step_id: fixture.implementStep.id,
          replacement_profile_id: fixture.profileC.id,
        }),
      ]);

      const plannedBeforeImplement = await previewWorkflowMove(
        apiClient,
        replacementBTask.id,
        fixture.workflow.id,
        fixture.prStep.id,
      );
      expect(plannedBeforeImplement.model.after.label).toContain("mock-slow");
      expect(plannedBeforeImplement.recipient?.profile_id).toBe(fixture.profileB.id);

      const implementSessionIds = new Map<string, string>();
      for (const [task, profile] of [
        [replacementBTask, fixture.profileB],
        [defaultTask, fixture.profileA],
        [replacementCTask, fixture.profileC],
      ] as const) {
        await apiClient.moveTask(task.id, fixture.workflow.id, fixture.implementStep.id);
        await waitForWorkflowStep(apiClient, task.id, fixture.implementStep.id);
        implementSessionIds.set(
          task.id,
          await waitForNewWorkflowProfileSession(apiClient, task.id, profile.id, initialSessionIds),
        );
        await expect
          .poll(() => apiClient.getTask(task.id).then((current) => current.primary_session_id), {
            timeout: 30_000,
            message: `task ${task.title} did not promote its Implement session`,
          })
          .toBe(implementSessionIds.get(task.id));
        await waitForWorkflowMoveLifecycle(apiClient, task.id);
      }

      for (const task of [replacementCTask, defaultTask, replacementBTask]) {
        await apiClient.moveTask(task.id, fixture.workflow.id, fixture.reviewStep.id);
        await waitForWorkflowStep(apiClient, task.id, fixture.reviewStep.id);
        await expect
          .poll(() => apiClient.getTask(task.id).then((current) => current.primary_session_id))
          .toBe(initialSessionIds[tasks.indexOf(task)]);
        await waitForWorkflowMoveLifecycle(apiClient, task.id);
      }

      for (const task of [replacementBTask, defaultTask, replacementCTask]) {
        await apiClient.moveTask(task.id, fixture.workflow.id, fixture.prStep.id);
        await waitForWorkflowStep(apiClient, task.id, fixture.prStep.id);
        await expect
          .poll(
            async () => {
              const [current, { sessions }] = await Promise.all([
                apiClient.getTask(task.id),
                apiClient.listTaskSessions(task.id),
              ]);
              const expected = implementSessionIds.get(task.id);
              return current.primary_session_id === expected
                ? expected
                : `primary=${current.primary_session_id}; expected=${expected}; overrides=${JSON.stringify(current.workflow_agent_overrides)}; sessions=${JSON.stringify(sessions.map((session) => ({ id: session.id, profile: session.agent_profile_id, state: session.state, primary: session.is_primary })))}`;
            },
            { timeout: 30_000 },
          )
          .toBe(implementSessionIds.get(task.id));
        await waitForWorkflowMoveLifecycle(apiClient, task.id);
      }

      const sessionPage = new SessionPage(testPage);
      for (const task of tasks) {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        if (sessions.length !== 2) {
          throw new Error(
            `task ${task.id} has ${sessions.length} sessions before reload: ${JSON.stringify(
              sessions.map((session) => ({
                id: session.id,
                profile: session.agent_profile_id,
                state: session.state,
                primary: session.is_primary,
              })),
            )}`,
          );
        }
        await testPage.goto(`/t/${task.id}`);
        await sessionPage.waitForLoad();
        const sessionTabs = testPage.locator(
          "[data-testid^='session-tab-']:not([data-testid^='session-tab-close-'])",
        );
        await expect(sessionTabs).toHaveCount(2, {
          timeout: 30_000,
        });
        await expect(
          sessionPage.sessionTabBySessionId(initialSessionIds[tasks.indexOf(task)]),
        ).toHaveCount(1);
        await expect(
          sessionPage.sessionTabBySessionId(implementSessionIds.get(task.id)!),
        ).toHaveCount(1);
        await testPage.reload();
        await sessionPage.waitForLoad();
        await expect(sessionTabs).toHaveCount(2, {
          timeout: 30_000,
        });
        await expect(
          sessionPage.sessionTabBySessionId(initialSessionIds[tasks.indexOf(task)]),
        ).toHaveCount(1);
        await expect(
          sessionPage.sessionTabBySessionId(implementSessionIds.get(task.id)!),
        ).toHaveCount(1);
      }

      await apiClient.updateWorkflowStep(fixture.implementStep.id, {
        agent_profile_id: fixture.profileC.id,
      });
      const laterStep = await apiClient.createWorkflowStep(fixture.workflow.id, "Later", 4, {
        agent_profile_id: fixture.profileC.id,
      });
      const retainedB = await previewWorkflowMove(
        apiClient,
        replacementBTask.id,
        fixture.workflow.id,
        fixture.implementStep.id,
      );
      expect(retainedB.recipient?.profile_id).toBe(fixture.profileB.id);
      const newStep = await previewWorkflowMove(
        apiClient,
        replacementBTask.id,
        fixture.workflow.id,
        laterStep.id,
      );
      expect(newStep.recipient?.profile_id).toBe(fixture.profileC.id);
    } finally {
      await deleteFixtureTasks(
        apiClient,
        tasks.map((task) => task.id),
      );
      await apiClient.deleteWorkflow(fixture.workflow.id).catch(() => undefined);
    }
  });

  test("cold workflow metadata loads an explicitly selected unopened workflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 1440, height: 900 });
    const scenario = await seedColdWorkflowMetadataScenario(
      apiClient,
      seedData,
      "Cold Explicit Desktop",
    );
    const { fixture, boardWorkflow } = scenario;
    await saveColdWorkflowMetadataSettings(apiClient, seedData, scenario, false);
    const requests = recordWorkflowMetadataRequests(testPage);
    const createdTaskIds: string[] = [];

    try {
      const kanban = new KanbanPage(testPage);
      await kanban.goto(boardWorkflow.id);
      await waitForLoadedBoardSnapshot(testPage, boardWorkflow.id);
      expect(await hasWorkflowSnapshot(testPage, fixture.workflow.id)).toBe(false);
      expect(workflowMetadataRequests(requests, fixture.workflow.id, "snapshot")).toHaveLength(0);

      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("workflow-selector-trigger")).toContainText(
        boardWorkflow.name,
      );
      await selectWorkflow(testPage, dialog, fixture.workflow.name);
      await dialog.getByTestId("task-title-input").fill("Cold explicit desktop task");
      await dialog.getByTestId("task-description-input").fill("Cold explicit desktop description");
      await dialog.getByTestId("task-create-advanced-settings-trigger").click();

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

      await row.getByTestId("agent-profile-selector").click();
      await testPage
        .getByRole("option", { name: new RegExp(escapeRegExp(fixture.profileB.name)) })
        .click();
      await expect(row.getByTestId("agent-profile-selector")).toContainText(fixture.profileB.name);

      await dialog.getByTestId("submit-start-agent-chevron").click();
      await testPage.getByTestId("submit-create-without-agent").click();
      await expect(dialog).not.toBeVisible({ timeout: 15_000 });

      const createdTaskId = taskIdFromUrl(testPage.url());
      createdTaskIds.push(createdTaskId);
      const stored = await apiClient.getTask(createdTaskId);
      expect(stored.workflow_agent_overrides).toMatchObject({
        workflow_id: fixture.workflow.id,
        steps: [
          {
            step_id: fixture.implementStep.id,
            source_profile_id: fixture.profileA.id,
            replacement_profile_id: fixture.profileB.id,
          },
        ],
      });

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
    await testPage.setViewportSize({ width: 1440, height: 900 });
    const scenario = await seedColdWorkflowMetadataScenario(
      apiClient,
      seedData,
      "Cold Remembered Desktop",
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
      const kanban = new KanbanPage(testPage);
      await kanban.goto(boardWorkflow.id);
      await waitForLoadedBoardSnapshot(testPage, boardWorkflow.id);
      expect(await hasWorkflowSnapshot(testPage, fixture.workflow.id)).toBe(false);

      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("workflow-selector-trigger")).toContainText(
        fixture.workflow.name,
      );
      await dialog.getByTestId("task-title-input").fill("Cold remembered desktop task");
      await dialog
        .getByTestId("task-description-input")
        .fill("Cold remembered desktop description");
      await dialog.getByTestId("task-create-advanced-settings-trigger").click();

      const overrides = dialog.getByTestId("task-create-workflow-agent-overrides");
      await expect(overrides.getByTestId("task-create-workflow-agent-overrides-retry")).toBeVisible(
        {
          timeout: 20_000,
        },
      );
      await expect(dialog.getByTestId("task-title-input")).toHaveValue(
        "Cold remembered desktop task",
      );

      await overrides.getByTestId("task-create-workflow-agent-overrides-retry").click();
      const row = overrides.getByTestId(
        `task-create-workflow-agent-override-${fixture.profileA.id}`,
      );
      await expect(row).toBeVisible({ timeout: 20_000 });
      await expect(row).toContainText("Implement");
      await expect(row).toContainText("PR");
      await row.getByTestId("agent-profile-selector").click();
      await testPage
        .getByRole("option", { name: new RegExp(escapeRegExp(fixture.profileB.name)) })
        .click();

      await dialog.getByTestId("submit-start-agent-chevron").click();
      await testPage.getByTestId("submit-create-without-agent").click();
      await expect(dialog).not.toBeVisible({ timeout: 15_000 });

      const createdTaskId = taskIdFromUrl(testPage.url());
      createdTaskIds.push(createdTaskId);
      const stored = await apiClient.getTask(createdTaskId);
      expect(stored.title).toBe("Cold remembered desktop task");
      expect(stored.description).toContain("Cold remembered desktop description");
      expect(stored.workflow_agent_overrides).toMatchObject({
        workflow_id: fixture.workflow.id,
        steps: [
          {
            step_id: fixture.implementStep.id,
            replacement_profile_id: fixture.profileB.id,
          },
        ],
      });
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
