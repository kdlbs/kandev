import { test, expect } from "../../fixtures/test-base";
import { expectCancelToSettlePromptly } from "../../helpers/cancellation";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

async function tapCancelButton(session: SessionPage) {
  const button = session.activeChat().getByTestId("cancel-agent-button");
  await expect(button).toBeVisible();
  await expect(button).toBeEnabled();
  await expect(button).toBeInViewport();
  // Mobile Chromium can finish a compositor/hydration frame after the
  // locator is visible. Retry the touch dispatch until the synchronous React
  // cancelling state is observable; the two-second responsiveness assertion
  // starts only after that state proves the request was accepted.
  await expect(async () => {
    await button.scrollIntoViewIfNeeded();
    await button.tap();
    await expect
      .poll(
        async () => {
          if ((await button.count()) === 0) return true;
          return button.isDisabled();
        },
        { timeout: 1_000 },
      )
      .toBe(true);
  }).toPass({ timeout: 6_000, intervals: [100, 250, 500] });
}

async function seedMobileCancellationWorkflow(apiClient: ApiClient, workspaceId: string) {
  const workflow = await apiClient.createWorkflow(workspaceId, "Mobile Cancel Completion");
  const inbox = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0, {
    is_start_step: true,
  });
  const working = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
  const done = await apiClient.createWorkflowStep(workflow.id, "Done", 2);
  await apiClient.updateWorkflowStep(working.id, {
    prompt:
      'e2e:message("mobile cancelable turn started")\ne2e:delay(8000)\ne2e:message("mobile cancelled turn marker")\n{{task_prompt}}',
    events: {
      on_enter: [{ type: "auto_start_agent" }],
      on_turn_complete: [{ type: "move_to_step", config: { step_id: done.id } }],
    },
    cancel_triggers_turn_complete: false,
  });
  return {
    workflowId: workflow.id,
    inboxStepId: inbox.id,
    workingStepId: working.id,
    doneStepId: done.id,
  };
}

test.describe("mobile: cancelled turn completion", () => {
  test.describe.configure({ retries: 1 });

  test("saves the policy by touch, reloads it, and advances after Cancel", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const workflow = await seedMobileCancellationWorkflow(apiClient, seedData.workspaceId);
    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);

    const card = await settings.findWorkflowCard("Mobile Cancel Completion");
    await settings.setCancelCompletionPolicy(card, "Working", true, true);
    const tappableLabel = card.getByTestId(`${workflow.workingStepId}-cancel-completion-label`);
    const labelBox = await tappableLabel.boundingBox();
    expect(labelBox).not.toBeNull();
    expect(labelBox!.height).toBeGreaterThanOrEqual(44);
    const signalRow = card.getByTestId(`${workflow.workingStepId}-require-signal-row`);
    const cancelRow = card.getByTestId(`${workflow.workingStepId}-cancel-completion-row`);
    const helpTip = card.getByTestId(`${workflow.workingStepId}-cancel-completion-help`);
    const [signalRowBox, cancelRowBox, helpBox] = await Promise.all([
      signalRow.boundingBox(),
      cancelRow.boundingBox(),
      helpTip.boundingBox(),
    ]);
    expect(signalRowBox).not.toBeNull();
    expect(cancelRowBox).not.toBeNull();
    expect(helpBox).not.toBeNull();
    expect(cancelRowBox!.height).toBeCloseTo(signalRowBox!.height, 1);
    expect(helpBox!.x - (labelBox!.x + labelBox!.width)).toBeGreaterThanOrEqual(0);
    expect(helpBox!.x - (labelBox!.x + labelBox!.width)).toBeLessThanOrEqual(12);
    await settings.saveChanges(true);

    await settings.goto(seedData.workspaceId);
    const reloadedCard = await settings.findWorkflowCard("Mobile Cancel Completion");
    const reloadedPanel = await settings.selectStep(reloadedCard, "Working", true);
    await expect(
      reloadedPanel.getByRole("checkbox", {
        name: "Run completion actions when a turn is cancelled",
      }),
    ).toBeChecked();
    expect(
      (await apiClient.listWorkflowSteps(workflow.workflowId)).steps.find(
        (step) => step.id === workflow.workingStepId,
      )?.cancel_triggers_turn_complete,
    ).toBe(true);

    const task = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile enabled cancellation task",
      {
        description: "Mobile cancellation routing test",
        workflow_id: workflow.workflowId,
        workflow_step_id: workflow.inboxStepId,
        agent_profile_id: seedData.agentProfileId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.moveTask(task.id, workflow.workflowId, workflow.workingStepId);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id, {
        timeout: 15_000,
      })
      .toBe(workflow.workingStepId);
    await expect(session.cancelAgentButton()).toBeVisible({ timeout: 30_000 });
    await expect(
      session.activeChat().getByText("mobile cancelable turn started", { exact: false }),
    ).toBeVisible({ timeout: 15_000 });

    expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(1);
    await tapCancelButton(session);
    await expectCancelToSettlePromptly(session);
    await expect
      .poll(async () => (await apiClient.getTask(task.id)).workflow_step_id, {
        timeout: 30_000,
      })
      .toBe(workflow.doneStepId);
    expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(1);
    await assertNoDocumentHorizontalOverflow(testPage);
  });
});
