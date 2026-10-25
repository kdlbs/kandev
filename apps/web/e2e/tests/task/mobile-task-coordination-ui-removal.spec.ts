import { expect, test } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import {
  rejectSessionEnsureRequests,
  routeSessionEntryRecovery,
} from "../../helpers/session-entry-recovery";
import type { Locator, Page } from "@playwright/test";

async function expectPhoneFeedbackBelowHeader(
  testPage: Page,
  feedback: Locator,
  recoveryTarget: Locator,
) {
  const layout = testPage.getByTestId("mobile-task-layout");
  const header = layout.locator(":scope > div.fixed.top-0");
  await expect(header).toBeVisible();
  await expect(feedback).toBeVisible();
  await expect(recoveryTarget).toBeVisible();
  await expect(recoveryTarget).toBeInViewport();

  const [headerBox, feedbackBox, targetBox] = await Promise.all([
    header.boundingBox(),
    feedback.boundingBox(),
    recoveryTarget.boundingBox(),
  ]);
  expect(headerBox).not.toBeNull();
  expect(feedbackBox).not.toBeNull();
  expect(targetBox).not.toBeNull();
  if (!headerBox || !feedbackBox || !targetBox) {
    throw new Error("phone recovery feedback geometry is unavailable");
  }
  const headerBottom = headerBox.y + headerBox.height;
  expect(feedbackBox.y).toBeGreaterThanOrEqual(headerBottom);
  expect(targetBox.y).toBeGreaterThanOrEqual(headerBottom);
}

test("phone task chat keeps its header clearance and omits coordination controls", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile task detail without coordination controls",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const claimResponse = await apiClient.rawRequest(
    "POST",
    `/api/v1/_test/tasks/${task.id}/management-claim`,
    { installation_id: "e2e-coordinator", instance_key: "configured-mobile-task" },
  );
  expect(claimResponse.ok).toBe(true);
  const criteriaResponse = await apiClient.rawRequest(
    "PUT",
    `/api/v1/tasks/${task.id}/completion-gate/criteria`,
    {
      expected_revision: 0,
      criteria: [
        {
          id: "required-review",
          description: "Review the task before completion",
          evidence_subject: { kind: "task_revision", id: task.id },
        },
      ],
    },
  );
  expect(criteriaResponse.ok).toBe(true);
  const coordinationReads: string[] = [];
  testPage.on("request", (request) => {
    const pathname = new URL(request.url()).pathname;
    if (
      pathname.startsWith(`/api/v1/tasks/${task.id}/management-claim`) ||
      pathname.startsWith(`/api/v1/tasks/${task.id}/completion-gate`)
    ) {
      coordinationReads.push(pathname);
    }
  });

  try {
    await testPage.goto(`/t/${task.id}`);
    const layout = testPage.getByTestId("mobile-task-layout");
    await expect(layout).toBeVisible();
    await expect(testPage.getByTestId("mobile-task-control-toolbar")).toHaveCount(0);
    await expect(testPage.getByTestId("task-management-claim-row")).toHaveCount(0);
    await expect(testPage.getByTestId("task-completion-gate-row")).toHaveCount(0);

    const topbar = layout.getByTestId("mobile-task-picker-trigger");
    const sessions = layout.getByTestId("mobile-sessions-pill");
    const editor = testPage.getByTestId("chat-input-editor");
    const submit = testPage.getByTestId("submit-message-button");
    const bottomNav = layout.getByTestId("session-mobile-bottom-nav");
    await expect(topbar).toBeVisible();
    await expect(sessions).toBeVisible();
    await expect(editor).toBeVisible();
    await expect(submit).toBeVisible();
    await expect(bottomNav).toBeVisible();

    const [sessionsBox, submitBox, bottomNavBox] = await Promise.all([
      sessions.boundingBox(),
      submit.boundingBox(),
      bottomNav.boundingBox(),
    ]);
    expect(sessionsBox).not.toBeNull();
    expect(submitBox).not.toBeNull();
    expect(bottomNavBox).not.toBeNull();
    if (!sessionsBox || !submitBox || !bottomNavBox) {
      throw new Error("phone task layout geometry is unavailable");
    }
    const header = layout.locator(":scope > div.fixed.top-0");
    const headerBox = await header.boundingBox();
    expect(headerBox).not.toBeNull();
    if (!headerBox) throw new Error("phone task header geometry is unavailable");
    const headerBottom = headerBox.y + headerBox.height;
    expect(sessionsBox.y - headerBottom).toBeGreaterThanOrEqual(0);
    expect(sessionsBox.y - headerBottom).toBeLessThanOrEqual(24);
    expect(submitBox.y + submitBox.height).toBeLessThanOrEqual(bottomNavBox.y + 1);
    const documentWidth = await testPage.evaluate(() => document.documentElement.scrollWidth);
    const viewportWidth = await testPage.evaluate(() => window.innerWidth);
    expect(documentWidth).toBeLessThanOrEqual(viewportWidth);
    await dwell(
      testPage,
      500,
      "negative-assertion",
      "removed phone task controls must not request claim or completion-gate data",
    );
    expect(coordinationReads).toEqual([]);
  } finally {
    await apiClient.deleteTask(task.id).catch(() => undefined);
  }
});

test("phone ensure-session failure is below the fixed header and retry is tappable", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const { task_id: taskId } = await apiClient.seedTask(
    seedData.workspaceId,
    "Mobile ensure-session error feedback",
  );
  const taskSessions = await apiClient.listTaskSessions(taskId);
  expect(taskSessions.sessions).toHaveLength(0);
  const ensureFailure = await rejectSessionEnsureRequests(
    testPage,
    "simulated session ensure failure",
  );

  try {
    await testPage.goto(`/t/${taskId}`);
    await expect.poll(ensureFailure.requestCount).toBeGreaterThan(0);
    const feedback = testPage.getByTestId("ensure-session-error-banner");
    const retry = testPage.getByTestId("ensure-session-error-retry");
    await expect(feedback).toBeVisible({ timeout: 45_000 });
    await expect(testPage.getByTestId("task-shared-error")).toHaveCount(0);
    await expectPhoneFeedbackBelowHeader(testPage, feedback, retry);

    await retry.tap();
    await expect.poll(ensureFailure.requestCount).toBeGreaterThan(1);
  } finally {
    await apiClient.deleteTask(taskId).catch(() => undefined);
  }
});

test("phone status-unavailable feedback is below the fixed header and retry is tappable", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile status unavailable feedback",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const recovery = await routeSessionEntryRecovery(testPage);
  recovery.dropNextResponses("task.session.status", 2);

  try {
    await testPage.goto(`/t/${task.id}`);
    const feedback = testPage.getByTestId("session-status-unavailable");
    const retry = testPage.getByTestId("session-status-retry");
    await expect(feedback).toBeVisible({ timeout: 45_000 });
    expect(recovery.droppedResponseCount("task.session.status")).toBe(2);
    await expect(testPage.getByTestId("task-shared-error")).toHaveCount(0);
    await expectPhoneFeedbackBelowHeader(testPage, feedback, retry);
    await retry.tap();
    await expect.poll(() => recovery.requestCount("task.session.status")).toBeGreaterThan(2);
  } finally {
    await apiClient.deleteTask(task.id).catch(() => undefined);
  }
});
