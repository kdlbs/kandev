import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForLatestSessionDone } from "../../helpers/session";
import { ChangeWorkflowPage } from "../../pages/change-workflow-page";
import { ThreadActionsPage } from "./threads-task-actions-helpers";
import {
  seedWorkflowAgentOverrideFixture,
  waitForNewWorkflowProfileSession,
  waitForWorkflowMoveLifecycle,
  waitForWorkflowStep,
} from "./task-workflow-agent-overrides-helpers";

test("changes workflow from phone task actions with task-local agent routing", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, testInfo) => {
  test.setTimeout(180_000);
  await testPage.setViewportSize({ width: 360, height: 640 });
  const fixture = await seedWorkflowAgentOverrideFixture(
    apiClient,
    seedData,
    "Change Workflow Phone",
  );
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Phone change workflow task",
    fixture.profileA.id,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  await waitForLatestSessionDone(apiClient, task.id, 1, "phone change workflow source task");
  const originalSessionId = await waitForNewWorkflowProfileSession(
    apiClient,
    task.id,
    fixture.profileA.id,
  );
  const before = await apiClient.getTask(task.id);
  await testPage.goto(`/threads?workspace=${seedData.workspaceId}&taskId=${task.id}`);
  const actions = new ThreadActionsPage(testPage, true);
  await expect(actions.trigger(task.id)).toBeVisible();
  await actions.expectHeaderActionAlignment(task.id);

  const openChangeForm = async () => {
    await actions.open(task.id);
    await expect(actions.choice("Change workflow...")).toBeVisible();
    await actions.pick("Change workflow...");
  };
  const chooseDraft = async () => {
    const form = new ChangeWorkflowPage(testPage, true);
    await expect(form.phoneDrawer).toBeVisible();
    await form.chooseWorkflow(fixture.workflow.id);
    await form.chooseStep(fixture.prStep.id);
    await form.chooseProfile(fixture.profileA.id, fixture.profileB.name);
    return form;
  };

  await openChangeForm();
  const form = await chooseDraft();
  await actions.contained(form.phoneDrawer);
  const scroll = form.form.getByTestId("change-workflow-scroll");
  await expect(scroll).toHaveCSS("overflow-y", "auto");
  expect(
    await form.phoneDrawer.evaluate(
      (element) =>
        [element, ...element.querySelectorAll("*")].filter((node) =>
          /auto|scroll/.test(getComputedStyle(node).overflowY),
        ).length,
    ),
  ).toBe(1);
  const footer = testPage.getByTestId("change-workflow-submit").locator("xpath=..");
  await expect(footer).toHaveClass(/safe-area-inset-bottom/);

  for (const selector of [
    form.form.getByTestId("change-workflow-destination"),
    form.form.getByTestId("change-workflow-step"),
    form.form.getByTestId(`change-workflow-profile-selector-${fixture.profileA.id}`),
    form.form.getByTestId("change-workflow-submit"),
  ]) {
    await selector.scrollIntoViewIfNeeded();
    const target = await selector.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      return {
        width: rect.width,
        height: rect.height,
        receivesTouch: element.contains(
          document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2),
        ),
      };
    });
    expect(target.width).toBeGreaterThanOrEqual(44);
    expect(target.height).toBeGreaterThanOrEqual(44);
    expect(target.receivesTouch).toBe(true);
  }
  await assertNoDocumentHorizontalOverflow(testPage, "phone change workflow form");
  await scroll.evaluate((element) => element.scrollTo({ top: 0 }));
  await testPage.screenshot({ path: testInfo.outputPath("phone-change-workflow-form.png") });
  if (prCapture.capturing) {
    await testPage.evaluate(async () => {
      await Promise.all(
        document
          .getAnimations()
          .filter((animation) => animation.playState === "running")
          .map((animation) => animation.finished.catch(() => undefined)),
      );
    });
    await prCapture.screenshot("phone-change-workflow-form", {
      caption: "Phone Change workflow form with contained scrolling and a fixed submit action.",
    });
  }
  await actions.press(form.form.getByTestId("change-workflow-cancel"));
  await expect(form.phoneDrawer).toBeHidden();
  await expect(actions.trigger(task.id)).toBeFocused();

  await openChangeForm();
  const submitForm = await chooseDraft();
  await submitForm.submit();
  await expect(submitForm.phoneDrawer).toBeHidden();
  await waitForWorkflowStep(apiClient, task.id, fixture.prStep.id);
  await waitForNewWorkflowProfileSession(apiClient, task.id, fixture.profileB.id, [
    originalSessionId,
  ]);
  await waitForWorkflowMoveLifecycle(apiClient, task.id);
  await expect(actions.column(task.id)).toBeVisible();
  await actions.open(task.id);
  await actions.nested("Move to");
  const currentDestinationStep = testPage.getByTestId(`task-context-step-${fixture.prStep.id}`);
  await expect(currentDestinationStep).toBeVisible();
  await expect(currentDestinationStep).toBeDisabled();
  if (prCapture.capturing) {
    await testPage.evaluate(async () => {
      await Promise.all(
        document
          .getAnimations()
          .filter((animation) => animation.playState === "running")
          .map((animation) => animation.finished.catch(() => undefined)),
      );
    });
    await prCapture.screenshot("phone-task-destination-step", {
      caption:
        "Phone task actions showing the task in its destination workflow step after migration.",
    });
  }

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
  expect(sessions.some((session) => session.id === originalSessionId)).toBe(true);
});
