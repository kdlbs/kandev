import { test, expect } from "../../fixtures/test-base";
import { ChangeWorkflowPage } from "../../pages/change-workflow-page";
import { KanbanPage } from "../../pages/kanban-page";
import {
  seedWorkflowAgentOverrideFixture,
  waitForNewWorkflowProfileSession,
  waitForWorkflowMoveLifecycle,
  waitForWorkflowStep,
} from "./task-workflow-agent-overrides-helpers";

test.describe("Change workflow", () => {
  test("maps a task profile for a later destination step and preserves its context", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    test.setTimeout(180_000);
    const fixture = await seedWorkflowAgentOverrideFixture(
      apiClient,
      seedData,
      "Change Workflow Desktop",
    );
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Change workflow source task",
      fixture.profileA.id,
      {
        description: "Keep this task brief and repository context",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    const existingSessionId = await waitForNewWorkflowProfileSession(
      apiClient,
      task.id,
      fixture.profileA.id,
    );
    const before = await apiClient.getTask(task.id);
    const previewChanges: Array<Record<string, unknown>> = [];
    testPage.on("request", (request) => {
      if (request.url().includes(`/api/v1/tasks/${task.id}/move-preview`)) {
        const body = request.postDataJSON() as {
          workflow_change?: { agent_overrides?: Record<string, string> };
        };
        if (body.workflow_change)
          previewChanges.push(body.workflow_change as unknown as Record<string, unknown>);
      }
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.openTaskActionsMenu(task.id);

    await expect(testPage.getByRole("menuitem", { name: "Change workflow..." })).toBeVisible();
    await kanban.contextChangeWorkflow().click();
    const changeWorkflow = new ChangeWorkflowPage(testPage);
    await expect(changeWorkflow.desktopDialog).toBeVisible();
    await changeWorkflow.chooseWorkflow(fixture.workflow.id);
    await changeWorkflow.chooseStep(fixture.prStep.id);
    await changeWorkflow.chooseProfile(fixture.profileA.id, fixture.profileB.name);
    await expect
      .poll(() =>
        previewChanges.some((change) =>
          JSON.stringify(change.agent_overrides).includes(fixture.profileB.id),
        ),
      )
      .toBe(true);
    await expect(testPage.getByTestId("workflow-move-preview")).toBeVisible();
    const preview = testPage.getByTestId("workflow-move-preview");
    await expect(preview).toContainText("New session");
    await preview.getByTestId("workflow-move-preview-details-toggle").click();
    const previewDetails = testPage.getByTestId("workflow-move-preview-details");
    await expect(previewDetails).toContainText(fixture.profileB.name);
    await expect(previewDetails).toContainText("mock-slow");
    await changeWorkflow.form
      .getByTestId("change-workflow-scroll")
      .evaluate((element) => element.scrollTo({ top: 0 }));
    await testPage.screenshot({ path: testInfo.outputPath("desktop-change-workflow-form.png") });
    if (prCapture.capturing) {
      await testPage.evaluate(async () => {
        await Promise.all(
          document
            .getAnimations()
            .filter((animation) => animation.playState === "running")
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
      await prCapture.screenshot("desktop-change-workflow-form", {
        caption: "Desktop Change workflow form with its predicted destination recipient.",
      });
    }
    await changeWorkflow.submit();
    await expect(changeWorkflow.desktopDialog).toBeHidden();

    await waitForWorkflowStep(apiClient, task.id, fixture.prStep.id);
    await waitForNewWorkflowProfileSession(apiClient, task.id, fixture.profileB.id, [
      existingSessionId,
    ]);
    await waitForWorkflowMoveLifecycle(apiClient, task.id);
    const changed = await apiClient.getTask(task.id);
    expect(changed).toMatchObject({
      id: before.id,
      title: before.title,
      description: before.description,
      workflow_id: fixture.workflow.id,
      workflow_step_id: fixture.prStep.id,
      repositories: before.repositories,
      workflow_agent_overrides: {
        workflow_id: fixture.workflow.id,
        steps: [
          {
            step_id: fixture.implementStep.id,
            source_profile_id: fixture.profileA.id,
            replacement_profile_id: fixture.profileB.id,
          },
        ],
      },
    });
    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions.some((session) => session.id === existingSessionId)).toBe(true);
  });

  test("single-task context action opens the same form from right-click", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const targetWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Change workflow target",
    );
    await apiClient.createWorkflowStep(targetWorkflow.id, "Incoming", 0);
    const task = await apiClient.createTask(seedData.workspaceId, "Change workflow context task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.openTaskContextMenu(task.id);
    await expect(kanban.contextChangeWorkflow()).toBeVisible();
    await kanban.openChangeWorkflowForm();
    await expect(testPage.getByRole("dialog", { name: "Change workflow..." })).toBeVisible();
    await testPage.getByTestId("change-workflow-cancel").click();
  });
});
