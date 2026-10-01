import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { waitForSessionDone } from "../../helpers/session";
import { ChangeWorkflowPage } from "../../pages/change-workflow-page";
import { KanbanPage } from "../../pages/kanban-page";
import { seedRemainingStepColors } from "./change-workflow-color-helpers";
import {
  seedWorkflowAgentOverrideFixture,
  waitForNewWorkflowProfileSession,
  waitForWorkflowMoveLifecycle,
  waitForWorkflowStep,
} from "./task-workflow-agent-overrides-helpers";

test.describe("Change workflow", () => {
  // @covers AC-TASKS-CHANGE-WORKFLOW-001.9
  test("shows step colors in destination options and the selected value", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    const destination = await apiClient.createWorkflow(seedData.workspaceId, "Colored steps");
    const analysis = await apiClient.createWorkflowStep(destination.id, "Analysis", 0);
    const implement = await apiClient.createWorkflowStep(destination.id, "Implement", 1);
    for (const [id, color] of [
      [analysis.id, "bg-blue-500"],
      [implement.id, "bg-green-500"],
    ]) {
      expect(
        (await apiClient.rawRequest("PUT", `/api/v1/workflow/steps/${id}`, { color })).ok,
      ).toBe(true);
    }
    const remainingColors = await seedRemainingStepColors(apiClient, destination.id);
    const other = await apiClient.createWorkflow(seedData.workspaceId, "Another destination");
    const incoming = await apiClient.createWorkflowStep(other.id, "Incoming", 0);
    const task = await apiClient.createTask(seedData.workspaceId, "Step color task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto(seedData.workflowId);
    await kanban.openTaskActionsMenu(task.id);
    await kanban.openChangeWorkflowForm();
    const form = new ChangeWorkflowPage(testPage);
    await expect(form.desktopDialog).toBeVisible();
    await form.chooseWorkflow(destination.id);
    await form.expectStepOptionColor(analysis.id, "var(--color-blue-500)");
    await form.expectStepOptionColor(implement.id, "var(--color-green-500)");
    await testPage.screenshot({
      path: testInfo.outputPath("desktop-step-colors.png"),
      animations: "disabled",
    });
    if (prCapture.capturing) {
      await waitForFiniteAnimations(testPage.locator("body"));
      await prCapture.screenshot("desktop-step-colors", {
        caption: "Destination step colors in the desktop Change workflow picker",
      });
    }
    // Reviewer-requested coverage of the existing palette and fallback CSS.
    for (const step of remainingColors) {
      await form.expectStepOptionColor(step.id, step.cssColor);
      await form.chooseStep(step.id);
      await form.expectSelectedStepColor(step.cssColor);
    }
    await form.chooseStep(analysis.id);
    await form.expectSelectedStepColor("var(--color-blue-500)");
    await form.expectStepOptionColor(implement.id, "var(--color-green-500)");
    await testPage.getByPlaceholder("Search steps...").fill("Implement");
    await expect(testPage.getByRole("option", { name: "Analysis", exact: true })).toBeHidden();
    await form.chooseStep(implement.id);
    await form.expectSelectedStepColor("var(--color-green-500)");
    await form.chooseWorkflow(other.id);
    const stepTrigger = form.form.getByTestId("change-workflow-step");
    await expect(stepTrigger).toContainText("Select a step");
    await expect(stepTrigger.locator(".rounded-full")).toHaveCount(0);
    await form.chooseStep(incoming.id);
    await form.submit();
    await expect(form.desktopDialog).toBeHidden();
    await waitForWorkflowStep(apiClient, task.id, incoming.id);
    expect((await apiClient.getTask(task.id)).workflow_id).toBe(other.id);
  });

  test("updates an open task page after a move from another client", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const destination = await apiClient.createWorkflow(
      seedData.workspaceId,
      "External move target",
    );
    const analysis = await apiClient.createWorkflowStep(destination.id, "Analysis", 0);
    const implement = await apiClient.createWorkflowStep(destination.id, "Implement", 1);
    const task = await apiClient.createTask(seedData.workspaceId, "External workflow move task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${task.id}`);
    await expect(testPage.getByTestId("task-topbar")).toBeVisible();
    const taskUrl = testPage.url();

    await apiClient.moveTask(task.id, destination.id, analysis.id);

    await expect(testPage).toHaveURL(taskUrl);
    const stepper = testPage.getByTestId("workflow-stepper");
    await expect(stepper.getByTestId("workflow-step-Analysis")).toHaveAttribute(
      "aria-current",
      "step",
    );
    const fullImplementStep = stepper.getByTestId("workflow-step-Implement");
    const compactStepper = stepper.getByTestId("workflow-stepper-minimal");
    await expect
      .poll(
        async () => (await fullImplementStep.count()) > 0 || (await compactStepper.count()) > 0,
        { timeout: 15_000, message: "the destination workflow stepper did not render" },
      )
      .toBe(true);
    if (await fullImplementStep.count()) {
      await expect(fullImplementStep).toBeVisible();
    } else {
      // The responsive top bar can collapse the stepper under shard viewport
      // pressure. The same step list is then available from its disclosure.
      await expect(compactStepper).toBeVisible();
      await compactStepper.hover();
      const disclosure = testPage.getByTestId("workflow-step-disclosure");
      await expect(disclosure).toBeVisible();
      await expect(
        disclosure.getByTestId(`workflow-step-disclosure-row-${implement.id}`),
      ).toBeVisible();
    }
  });

  test("updates the open task stepper after changing workflow and preserves its context", async ({
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
    await testPage.goto(`/t/${task.id}`);
    await expect(testPage.getByTestId("task-topbar")).toBeVisible();
    const taskUrl = testPage.url();
    await testPage.getByTestId("task-topbar-actions-menu").click();
    // The menu label follows the current product wording, while this stable
    // test id identifies the single-task workflow action across both labels.
    await expect(testPage.getByTestId("task-context-change-workflow")).toBeVisible();
    await testPage.getByTestId("task-context-change-workflow").click();

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
    await expect(testPage).toHaveURL(taskUrl);
    const stepper = testPage.getByTestId("workflow-stepper");
    await expect(stepper.getByTestId("workflow-step-PR")).toHaveAttribute("aria-current", "step");
    const destinationSteps = [
      { id: fixture.analysisStep.id, name: "Analysis" },
      { id: fixture.implementStep.id, name: "Implement" },
      { id: fixture.reviewStep.id, name: "Review" },
      { id: fixture.prStep.id, name: "PR" },
    ];
    const analysisStep = stepper.getByTestId("workflow-step-Analysis");
    if ((await analysisStep.count()) > 0) {
      await expect(analysisStep).toBeVisible();
      await expect(stepper.getByTestId("workflow-step-Implement")).toBeVisible();
      await expect(stepper.getByTestId("workflow-step-Review")).toBeVisible();
    } else {
      const compactTrigger = stepper.getByTestId("workflow-stepper-minimal");
      await expect(compactTrigger).toBeVisible();
      await compactTrigger.hover();
      const disclosure = testPage.getByTestId("workflow-step-disclosure");
      await expect(disclosure).toBeVisible();
      for (const step of destinationSteps) {
        const row = disclosure.getByTestId(`workflow-step-disclosure-row-${step.id}`);
        await expect(row).toBeVisible();
        await expect(row).toContainText(step.name);
      }
      await expect(
        disclosure.getByTestId(`workflow-step-disclosure-row-${fixture.prStep.id}`),
      ).toHaveAttribute("aria-current", "step");
      await testPage.keyboard.press("Escape");
    }
    if (prCapture.capturing) {
      await testPage.evaluate(async () => {
        await Promise.all(
          document
            .getAnimations()
            .filter((animation) => animation.playState === "running")
            .map((animation) => animation.finished.catch(() => undefined)),
        );
      });
      await prCapture.screenshot("desktop-task-destination-stepper", {
        caption:
          "Open task page showing the destination workflow and current step after migration.",
      });
    }
    const destinationSessionId = await waitForNewWorkflowProfileSession(
      apiClient,
      task.id,
      fixture.profileB.id,
      [existingSessionId],
    );
    await waitForWorkflowMoveLifecycle(apiClient, task.id);
    const { session: routedSession } = await apiClient.getTaskSession(destinationSessionId);
    await waitForSessionDone(
      apiClient,
      task.id,
      existingSessionId,
      "source workflow session must settle before cleanup",
      30_000,
    );
    await waitForSessionDone(
      apiClient,
      task.id,
      destinationSessionId,
      "destination workflow session must settle before cleanup",
      30_000,
    );
    expect(routedSession.agent_profile_id).toBe(fixture.profileB.id);
    expect(routedSession.agent_profile_snapshot?.model).toBe("mock-slow");
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

test.describe("coarse-pointer tablet Change workflow", () => {
  test.use({ hasTouch: true, viewport: { width: 900, height: 1000 } });

  test("keeps touch-sized form controls in the desktop dialog", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const destination = await apiClient.createWorkflow(seedData.workspaceId, "Tablet target");
    const step = await apiClient.createWorkflowStep(destination.id, "Incoming", 0);
    const task = await apiClient.createTask(seedData.workspaceId, "Tablet change workflow task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.openTaskActionsMenu(task.id);
    await expect(kanban.contextChangeWorkflow()).toBeVisible();
    await kanban.openChangeWorkflowForm();

    const form = new ChangeWorkflowPage(testPage);
    await expect(form.desktopDialog).toBeVisible();
    await expect(form.phoneDrawer).toHaveCount(0);
    expect(await testPage.evaluate(() => window.innerWidth)).toBe(900);
    expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
    await form.chooseWorkflow(destination.id);
    await form.chooseStep(step.id);

    for (const control of [
      testPage.getByTestId("change-workflow-close"),
      form.form.getByTestId("change-workflow-destination"),
      form.form.getByTestId("change-workflow-step"),
      form.form.getByTestId("change-workflow-submit"),
    ]) {
      const box = await control.boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(44);
      const receivesPointer = await control.evaluate((element) => {
        const rect = element.getBoundingClientRect();
        return element.contains(
          document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2),
        );
      });
      expect(receivesPointer).toBe(true);
    }
  });
});
