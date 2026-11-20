import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { Page } from "@playwright/test";

type TaskStart = { id: string; session_id?: string };
type BlockedKanbanTaskOptions = {
  workspaceId: string;
  workflowId: string;
  agentProfileId: string;
  title: string;
  blockerIds: string[];
  response: string;
};

async function createKanbanWorkflow(apiClient: ApiClient, workspaceId: string): Promise<string> {
  const workflow = await apiClient.createWorkflow(
    workspaceId,
    `Office state events ${Date.now()}`,
    "simple",
  );
  return workflow.id;
}

async function seedOfficePredecessor(
  apiClient: ApiClient,
  workflowId: string,
  workspaceId: string,
  title: string,
): Promise<TaskStart> {
  const { steps } = await apiClient.listWorkflowSteps(workflowId);
  const terminalStep = steps.reduce((max, step) => (step.position > max.position ? step : max));
  const seeded = await apiClient.seedTask(workspaceId, title, {
    workflow_id: workflowId,
    workflow_step_id: terminalStep.id,
    state: "TODO",
  });
  return { id: seeded.task_id };
}

async function createBlockedKanbanTask(
  apiClient: ApiClient,
  options: BlockedKanbanTaskOptions,
): Promise<TaskStart> {
  const task = await apiClient.createTaskWithAgent(
    options.workspaceId,
    options.title,
    options.agentProfileId,
    {
      description: `e2e:message(${JSON.stringify(options.response)})`,
      workflow_id: options.workflowId,
      blocked_by: options.blockerIds,
      start_when_unblocked: true,
    },
  );
  expect(
    task.session_id,
    "a dependent must be sessionless while its blockers are unresolved",
  ).toBeFalsy();
  expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(0);
  return task;
}

async function markOfficeTaskDone(testPage: Page, taskId: string) {
  const saved = waitForHttp(testPage, "PATCH", new RegExp(`/api/v1/office/tasks/${taskId}$`), {
    predicate: (response) => response.ok(),
  });
  await testPage.getByTestId("status-picker-trigger").click();
  await testPage.getByTestId("status-picker-option-done").click();
  await saved;
  await expect(testPage.getByTestId("status-picker-trigger")).toContainText(/Done/i, {
    timeout: 15_000,
  });
}

async function expectSessionCount(apiClient: ApiClient, taskId: string, count: number) {
  await expect
    .poll(async () => (await apiClient.listTaskSessions(taskId)).total, {
      timeout: 30_000,
      message: `task ${taskId} should have ${count} session(s)`,
    })
    .toBe(count);
}

test.describe("Office task state events", () => {
  test("status picker starts a Kanban dependent once and preserves its visible response", async ({
    testPage,
    apiClient,
    seedData,
    officeSeed,
  }) => {
    test.setTimeout(120_000);
    const workflowId = await createKanbanWorkflow(apiClient, officeSeed.workspaceId);
    const predecessor = await seedOfficePredecessor(
      apiClient,
      officeSeed.workflowId,
      officeSeed.workspaceId,
      "Office state event predecessor",
    );
    const response = "office-state-event-dependent-response";
    const dependent = await createBlockedKanbanTask(apiClient, {
      workspaceId: officeSeed.workspaceId,
      workflowId,
      agentProfileId: seedData.agentProfileId,
      title: "Office state event dependent",
      blockerIds: [predecessor.id],
      response,
    });

    await testPage.goto(`/office/tasks/${predecessor.id}`);
    await expect(
      testPage.getByRole("heading", { name: "Office state event predecessor" }),
    ).toBeVisible({ timeout: 10_000 });
    await markOfficeTaskDone(testPage, predecessor.id);
    await expectSessionCount(apiClient, dependent.id, 1);

    const sessionPage = new SessionPage(testPage);
    await testPage.goto(`/t/${dependent.id}`);
    const responseMessage = sessionPage
      .activeChat()
      .getByTestId("agent-message-highlight")
      .filter({ hasText: response });
    await expect(responseMessage).toBeVisible({ timeout: 60_000 });

    const repeated = await apiClient.rawRequest("PATCH", `/api/v1/office/tasks/${predecessor.id}`, {
      status: "done",
    });
    expect(repeated.ok).toBe(true);
    await expectSessionCount(apiClient, dependent.id, 1);

    await testPage.reload();
    await expect(
      sessionPage.activeChat().getByTestId("agent-message-highlight").filter({ hasText: response }),
    ).toBeVisible({ timeout: 30_000 });
    expect((await apiClient.listTaskSessions(dependent.id)).sessions).toHaveLength(1);
  });

  test("a completed Office blocker waits for every remaining blocker", async ({
    testPage,
    apiClient,
    seedData,
    officeSeed,
  }) => {
    test.setTimeout(90_000);
    const workflowId = await createKanbanWorkflow(apiClient, officeSeed.workspaceId);
    const officeBlocker = await seedOfficePredecessor(
      apiClient,
      officeSeed.workflowId,
      officeSeed.workspaceId,
      "Office pending blocker",
    );
    const pendingBlocker = await apiClient.createTask(
      officeSeed.workspaceId,
      "Kanban pending blocker",
      {
        workflow_id: workflowId,
      },
    );
    const dependent = await createBlockedKanbanTask(apiClient, {
      workspaceId: officeSeed.workspaceId,
      workflowId,
      agentProfileId: seedData.agentProfileId,
      title: "Multiple blocker dependent",
      blockerIds: [officeBlocker.id, pendingBlocker.id],
      response: "multiple-blocker-response",
    });

    await testPage.goto(`/office/tasks/${officeBlocker.id}`);
    await expect(testPage.getByRole("heading", { name: "Office pending blocker" })).toBeVisible({
      timeout: 10_000,
    });
    await markOfficeTaskDone(testPage, officeBlocker.id);
    expect((await apiClient.getTaskDependencies(dependent.id)).blocked).toBe(true);
    expect((await apiClient.listTaskSessions(dependent.id)).sessions).toHaveLength(0);

    await apiClient.updateTaskState(pendingBlocker.id, "COMPLETED");
    await expectSessionCount(apiClient, dependent.id, 1);
  });

  test("an approval redirect to Review keeps the dependent blocked", async ({
    testPage,
    apiClient,
    officeApi,
    seedData,
    officeSeed,
  }) => {
    test.setTimeout(90_000);
    const workflowId = await createKanbanWorkflow(apiClient, officeSeed.workspaceId);
    const predecessor = await seedOfficePredecessor(
      apiClient,
      officeSeed.workflowId,
      officeSeed.workspaceId,
      "Approval redirected predecessor",
    );
    const dependent = await createBlockedKanbanTask(apiClient, {
      workspaceId: officeSeed.workspaceId,
      workflowId,
      agentProfileId: seedData.agentProfileId,
      title: "Approval redirected dependent",
      blockerIds: [predecessor.id],
      response: "approval-redirect-response",
    });
    const approver = (await officeApi.createAgent(officeSeed.workspaceId, {
      name: "State Event Approval Gate",
      role: "worker",
    })) as { id: string };
    const attached = await apiClient.rawRequest(
      "POST",
      `/api/v1/office/tasks/${predecessor.id}/approvers`,
      { agent_profile_id: approver.id },
    );
    expect(attached.ok).toBe(true);
    const movedToReview = await officeApi.rawRequest("PATCH", `/tasks/${predecessor.id}`, {
      status: "in_review",
    });
    expect(movedToReview.ok).toBe(true);

    await testPage.goto(`/office/tasks/${predecessor.id}`);
    await expect(
      testPage.getByRole("heading", { name: "Approval redirected predecessor" }),
    ).toBeVisible({ timeout: 10_000 });
    const doneRequest = waitForHttp(
      testPage,
      "PATCH",
      new RegExp(`/api/v1/office/tasks/${predecessor.id}$`),
    );
    await testPage.getByTestId("status-picker-trigger").click();
    await testPage.getByTestId("status-picker-option-done").click();
    const response = await doneRequest;
    expect(response.status()).toBe(409);
    await expect(testPage.getByTestId("status-picker-trigger")).toContainText(/In Review/i, {
      timeout: 15_000,
    });
    const persisted = (await officeApi.getTask(predecessor.id)) as Record<string, unknown>;
    const inner = (persisted.task as Record<string, unknown>) ?? persisted;
    expect(String(inner.status ?? inner.state).toLowerCase()).toContain("review");
    expect((await apiClient.getTaskDependencies(dependent.id)).blocked).toBe(true);
    expect((await apiClient.listTaskSessions(dependent.id)).sessions).toHaveLength(0);
  });
});
