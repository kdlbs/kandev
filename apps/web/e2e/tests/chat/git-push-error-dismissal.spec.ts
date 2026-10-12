import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

const FAILURE_TEXT = "Git push failed";
const DIAGNOSTIC = "remote rejected the branch";

async function seedDismissalTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: "dismiss one historical Git push failure",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, { state: "IDLE" });
  return { taskId: task.id, sessionId, title };
}

async function seedLegacyPushFailure(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  content = FAILURE_TEXT,
) {
  return apiClient.seedSessionMessage(sessionId, {
    type: "error",
    content,
    authorType: "agent",
    metadata: {
      git_operation_error: true,
      operation: "push",
      error_output: DIAGNOSTIC,
      session_id: sessionId,
      task_id: taskId,
      variant: "error",
      actions: [
        {
          type: "ws_request",
          label: "Fix",
          icon: "sparkles",
          test_id: "git-fix-button",
          params: {
            method: "message.add",
            payload: {
              task_id: taskId,
              session_id: sessionId,
              content: "Please fix the Git push error.",
            },
          },
        },
      ],
    },
  });
}

async function taskCard(session: SessionPage, text: string) {
  const card = session
    .activeChat()
    .getByTestId("session-recovery-action-message")
    .filter({ hasText: text });
  await expect(card).toBeVisible();
  return card;
}

async function switchDesktopTask(session: SessionPage, title: string, taskId: string) {
  const row = session.sidebar.getByTestId("sidebar-task-item").filter({ hasText: title });
  await expect(row).toBeVisible();
  await row.click();
  await expect(session.sidebar.page()).toHaveURL(new RegExp(`/t/${taskId}`));
  const next = new SessionPage(session.sidebar.page());
  await next.waitForLoad();
  await next.showSessionContext();
  return next;
}

test.describe("Git push error dismissal", () => {
  test("keeps one dismissal across reload and task switches while later failures remain visible", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const task = await seedDismissalTask(apiClient, seedData, `Git push dismissal ${Date.now()}`);
    const otherTask = await seedDismissalTask(apiClient, seedData, `Git push switch ${Date.now()}`);
    await seedLegacyPushFailure(apiClient, task.taskId, task.sessionId);

    let session = await openTaskSession(testPage, task.taskId);
    const card = await taskCard(session, FAILURE_TEXT);
    await expect(card.getByTestId("git-fix-button")).toBeVisible();
    await expect(card.getByTestId("git-push-error-dismiss-button")).toBeVisible();
    await prCapture.screenshot("before-dismiss");

    await card.getByTestId("git-push-error-dismiss-button").click();
    await expect(card).not.toBeVisible();

    const freshPage = await testPage.context().newPage();
    try {
      session = await openTaskSession(freshPage, task.taskId);
      await expect(
        session
          .activeChat()
          .getByTestId("session-recovery-action-message")
          .filter({ hasText: FAILURE_TEXT }),
      ).toHaveCount(0);
      await prCapture.screenshot("after-dismiss", { page: freshPage });
    } finally {
      await freshPage.close();
    }

    session = new SessionPage(testPage);
    await session.waitForLoad();
    session = await switchDesktopTask(session, otherTask.title, otherTask.taskId);
    session = await switchDesktopTask(session, task.title, task.taskId);
    await expect(
      session
        .activeChat()
        .getByTestId("session-recovery-action-message")
        .filter({ hasText: FAILURE_TEXT }),
    ).toHaveCount(0);

    await seedLegacyPushFailure(apiClient, task.taskId, task.sessionId, "Git push failed again");
    await testPage.reload();
    session = new SessionPage(testPage);
    await session.waitForLoad();
    const laterCard = await taskCard(session, "Git push failed again");
    await laterCard.getByText("Technical details", { exact: true }).click();
    await expect(laterCard.getByText(DIAGNOSTIC, { exact: true })).toBeVisible();
    await expect(laterCard.getByTestId("git-fix-button")).toBeVisible();
    await expect(laterCard.getByTestId("git-push-error-dismiss-button")).toBeVisible();
  });
});
