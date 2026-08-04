import { expect } from "@playwright/test";
import type { Page } from "@playwright/test";
import { test as e2eTest } from "../../fixtures/test-base";
import { ApiClient } from "../../helpers/api-client";
import { expectCancelToSettlePromptly } from "../../helpers/cancellation";
import { SessionPage } from "../../pages/session-page";

type CancellationWorkflow = {
  workflowId: string;
  inboxStepId: string;
  workingStepId: string;
  doneStepId: string;
};

async function createCancellationWorkflow(
  apiClient: ApiClient,
  workspaceId: string,
  enabled: boolean,
  name: string,
): Promise<CancellationWorkflow> {
  const workflow = await apiClient.createWorkflow(workspaceId, name);
  const inbox = await apiClient.createWorkflowStep(workflow.id, "Inbox", 0, {
    is_start_step: true,
  });
  const working = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
  const done = await apiClient.createWorkflowStep(workflow.id, "Done", 2);

  await apiClient.updateWorkflowStep(working.id, {
    prompt:
      'e2e:message("cancelable turn started")\ne2e:delay(8000)\ne2e:message("cancelled turn marker")\n{{task_prompt}}',
    events: {
      on_enter: [{ type: "auto_start_agent" }],
      on_turn_complete: [{ type: "move_to_step", config: { step_id: done.id } }],
    },
    cancel_triggers_turn_complete: enabled,
  });

  return {
    workflowId: workflow.id,
    inboxStepId: inbox.id,
    workingStepId: working.id,
    doneStepId: done.id,
  };
}

async function startWorkingTurn(
  testPage: Page,
  apiClient: ApiClient,
  seedData: { workspaceId: string; agentProfileId: string; repositoryId: string },
  workflow: CancellationWorkflow,
  title: string,
) {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: "Cancellation routing test",
    workflow_id: workflow.workflowId,
    workflow_step_id: workflow.inboxStepId,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });
  await apiClient.moveTask(task.id, workflow.workflowId, workflow.workingStepId);

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.stepperStep("Working")).toHaveAttribute("aria-current", "step", {
    timeout: 15_000,
  });
  await expect(session.agentStatus()).toBeVisible({ timeout: 30_000 });
  await expect(session.cancelAgentButton()).toBeVisible({ timeout: 15_000 });
  await expect(
    session.activeChat().getByText("cancelable turn started", { exact: false }),
  ).toBeVisible({
    timeout: 15_000,
  });
  return { task, session };
}

e2eTest.describe("Cancelled turn completion", () => {
  e2eTest.describe.configure({ retries: 1 });

  e2eTest(
    "enabled policy advances once after the real Cancel action",
    async ({ testPage, apiClient, seedData }) => {
      e2eTest.setTimeout(120_000);
      const workflow = await createCancellationWorkflow(
        apiClient,
        seedData.workspaceId,
        true,
        "Cancel Completion Enabled",
      );
      const { task, session } = await startWorkingTurn(
        testPage,
        apiClient,
        seedData,
        workflow,
        "Enabled cancellation task",
      );
      const sessionsBeforeCancel = await apiClient.listTaskSessions(task.id);
      expect(sessionsBeforeCancel.sessions).toHaveLength(1);

      await session.cancelAgentButton().click();
      await expectCancelToSettlePromptly(session);
      await expect(session.stepperStep("Done")).toHaveAttribute("aria-current", "step", {
        timeout: 30_000,
      });
      await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

      expect((await apiClient.getTask(task.id)).workflow_step_id).toBe(workflow.doneStepId);
      expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(1);
    },
  );

  e2eTest(
    "disabled policy keeps a cancelled turn in its current step",
    async ({ testPage, apiClient, seedData }) => {
      e2eTest.setTimeout(120_000);
      const workflow = await createCancellationWorkflow(
        apiClient,
        seedData.workspaceId,
        false,
        "Cancel Completion Disabled",
      );
      const { task, session } = await startWorkingTurn(
        testPage,
        apiClient,
        seedData,
        workflow,
        "Disabled cancellation task",
      );
      expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(1);

      await session.cancelAgentButton().click();
      await expectCancelToSettlePromptly(session);
      await expect(session.stepperStep("Working")).toHaveAttribute("aria-current", "step");

      expect((await apiClient.getTask(task.id)).workflow_step_id).toBe(workflow.workingStepId);
      expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(1);
    },
  );
});
