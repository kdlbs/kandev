import { test, expect } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { waitForHttp, watchWs } from "../../helpers/causal-waits";
import { ChangeWorkflowPage } from "../../pages/change-workflow-page";
import { KanbanPage } from "../../pages/kanban-page";
import { seedRemainingStepColors } from "./change-workflow-color-helpers";
import {
  seedWorkflowAgentOverrideFixture,
  waitForNewWorkflowProfileSession,
  waitForWorkflowProfileSessionCreated,
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

    const ws = watchWs(testPage);
    await testPage.goto(`/t/${task.id}`);
    await expect(testPage.getByTestId("task-topbar")).toBeVisible();
    const taskUrl = testPage.url();

    const moved = ws.waitForEvent("task.updated", {
      where: (payload) =>
        payload.task_id === task.id &&
        payload.workflow_id === destination.id &&
        payload.workflow_step_id === analysis.id,
    });
    await apiClient.moveTask(task.id, destination.id, analysis.id);
    await moved;

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
      ).toContainText("Implement");
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
    const [moveResponse] = await Promise.all([
      waitForHttp(testPage, "POST", new RegExp(`/api/v1/tasks/${task.id}/move$`)),
      changeWorkflow.submit(),
    ]);
    expect(moveResponse.ok()).toBe(true);
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
    const destinationSessionId = await waitForWorkflowProfileSessionCreated(
      apiClient,
      task.id,
      fixture.profileB.id,
      [existingSessionId],
    );
    await waitForWorkflowMoveLifecycle(apiClient, task.id);
    const { session: routedSession } = await apiClient.getTaskSession(destinationSessionId);
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

  // @covers AC-TASKS-CHANGE-WORKFLOW-001.10
  for (const theme of ["light", "dark"] as const) {
    test(`shows editable selector fields in ${theme}`, async ({
      testPage,
      apiClient,
      seedData,
    }, testInfo) => {
      test.setTimeout(180_000);
      const fixture = await seedWorkflowAgentOverrideFixture(
        apiClient,
        seedData,
        "Selector affordance with a deliberately long workflow name",
      );
      expect(
        (
          await apiClient.rawRequest("PUT", `/api/v1/workflow/steps/${fixture.analysisStep.id}`, {
            color: "bg-blue-500",
          })
        ).ok,
      ).toBe(true);
      const task = await apiClient.createTask(seedData.workspaceId, "Selector field task", {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      });
      await testPage.addInitScript((value) => localStorage.setItem("theme", value), theme);
      await testPage.setViewportSize({ width: 1440, height: 1000 });

      const kanban = new KanbanPage(testPage);
      await kanban.goto(seedData.workflowId);
      await expect(testPage.locator("html")).toHaveClass(new RegExp(`(^|\\s)${theme}(\\s|$)`));
      await kanban.openTaskActionsMenu(task.id);
      await kanban.openChangeWorkflowForm();
      const form = new ChangeWorkflowPage(testPage);
      await expect(form.desktopDialog).toBeVisible();
      await waitForFiniteAnimations(testPage.locator("body"));

      const unfocused = await form.readSelectorField("change-workflow-destination");
      form.expectVisibleSelectorField(unfocused, "destination workflow");
      await form.tabToSelectorField("change-workflow-destination");
      const destination = form.selectorField("change-workflow-destination");
      await expect(destination).toBeFocused();
      const focused = await form.readSelectorField("change-workflow-destination");
      form.expectVisibleSelectorField(focused, "focused destination workflow");
      form.expectStableSelectorBounds(unfocused, focused, "focused destination workflow");
      expect(focused.boxShadow, "keyboard focus should keep a visible ring").not.toBe("none");
      await destination.evaluate((element) => (element as HTMLElement).blur());

      await form.chooseWorkflow(fixture.workflow.id);
      const fields = [
        { id: "change-workflow-destination", label: "destination workflow" },
        { id: "change-workflow-step", label: "destination step" },
        {
          id: `change-workflow-profile-selector-${fixture.profileA.id}`,
          label: "replacement profile",
        },
      ];
      for (const field of fields) {
        const trigger = form.selectorField(field.id);
        await trigger.scrollIntoViewIfNeeded();
        await testPage.mouse.move(0, 0);
        await expect
          .poll(async () => (await form.readSelectorField(field.id)).backgroundAlpha)
          .toBe(255);
        const settled = await form.readSelectorField(field.id);
        form.expectVisibleSelectorField(settled, field.label);
        expect(settled.height, `${field.label} desktop height`).toBeCloseTo(28, 0);

        await trigger.hover();
        await expect
          .poll(async () => (await form.readSelectorField(field.id)).backgroundColor)
          .not.toBe(settled.backgroundColor);
        const hovered = await form.readSelectorField(field.id);
        form.expectVisibleSelectorField(hovered, `${field.label} hovered`, false);
        form.expectStableSelectorBounds(settled, hovered, `${field.label} hovered`);
        await testPage.mouse.move(0, 0);
        await expect
          .poll(async () => (await form.readSelectorField(field.id)).backgroundColor)
          .toBe(settled.backgroundColor);

        await trigger.click();
        await expect(trigger).toHaveAttribute("aria-expanded", "true");
        const expanded = await form.readSelectorField(field.id);
        form.expectVisibleSelectorField(expanded, `${field.label} expanded`, false);
        form.expectStableSelectorBounds(settled, expanded, `${field.label} expanded`);
        await testPage.keyboard.press("Escape");
        await expect(trigger).toHaveAttribute("aria-expanded", "false");
      }

      await form.chooseStep(fixture.analysisStep.id);
      await form.expectSelectedStepColor("var(--color-blue-500)");
      form.expectVisibleSelectorField(
        await form.readSelectorField("change-workflow-step"),
        "selected destination step",
      );
      await form.form
        .getByTestId("change-workflow-scroll")
        .evaluate((element) => element.scrollTo({ top: 0 }));
      await testPage.mouse.move(0, 0);
      await testPage.screenshot({
        path: testInfo.outputPath(`desktop-selector-fields-${theme}.png`),
        animations: "disabled",
      });

      const beforeSubmit = new Map<string, Awaited<ReturnType<typeof form.readSelectorField>>>();
      for (const field of fields) {
        beforeSubmit.set(field.id, await form.readSelectorField(field.id));
      }
      let releaseMove!: () => void;
      const moveHeld = new Promise<void>((resolve) => {
        releaseMove = resolve;
      });
      await testPage.route(`**/api/v1/tasks/${task.id}/move`, async (route) => {
        await moveHeld;
        await route.fulfill({
          status: 400,
          contentType: "application/json",
          body: JSON.stringify({ code: "invalid_workflow_change" }),
        });
      });
      await form.submit();
      await expect(form.form.getByTestId("change-workflow-submit")).toBeDisabled();
      for (const field of fields) {
        const trigger = form.selectorField(field.id);
        expect(await trigger.isDisabled(), `${field.label} should be disabled`).toBe(true);
        const disabled = await form.readSelectorField(field.id);
        form.expectVisibleSelectorField(disabled, `${field.label} disabled`, false);
        form.expectStableSelectorBounds(
          beforeSubmit.get(field.id)!,
          disabled,
          `${field.label} disabled`,
        );
        const opacity = await trigger.evaluate((element) =>
          Number.parseFloat(getComputedStyle(element).opacity),
        );
        expect(opacity, `${field.label} disabled feedback`).toBeLessThan(1);
      }
      releaseMove();
      await expect(form.form.getByTestId("change-workflow-submit")).toBeEnabled();
      await form.form.getByTestId("change-workflow-cancel").click();
      await expect(form.desktopDialog).toBeHidden();
    });
  }
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
