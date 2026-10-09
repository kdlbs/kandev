import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

const FAILURE_TEXT = "Git push failed";
const DIAGNOSTIC = "remote rejected the branch";
const MIN_TOUCH_TARGET_PX = 44;

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

async function switchMobileTask(session: SessionPage, title: string) {
  const page = session.activeChat().page();
  await page.getByTestId("mobile-task-picker-trigger").tap();
  const sheet = page.getByRole("dialog", { name: "Tasks" });
  const row = sheet.getByTestId("sidebar-task-item").filter({ hasText: title });
  await expect(row).toBeVisible();
  await row.tap();
  await expect(sheet).not.toBeVisible();
  const next = new SessionPage(page);
  await next.waitForLoad();
  return next;
}

test.describe("Mobile Git push error dismissal", () => {
  test("uses a touch-sized control and keeps later failures visible after reload and task switches", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const task = await seedDismissalTask(
      apiClient,
      seedData,
      `Mobile Git push dismissal ${Date.now()}`,
    );
    const otherTask = await seedDismissalTask(
      apiClient,
      seedData,
      `Mobile Git push switch ${Date.now()}`,
    );
    await seedLegacyPushFailure(apiClient, task.taskId, task.sessionId);

    let session = await openTaskSession(testPage, task.taskId);
    const card = session
      .activeChat()
      .getByTestId("session-recovery-action-message")
      .filter({ hasText: FAILURE_TEXT });
    await expect(card).toBeVisible();
    const dismiss = card.getByTestId("git-push-error-dismiss-button");
    await expect(dismiss).toBeVisible();
    await expect(card.getByTestId("git-fix-button")).toBeVisible();
    const touchBox = await dismiss.boundingBox();
    expect(touchBox).not.toBeNull();
    expect(touchBox!.height).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    expect(touchBox!.width).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    await prCapture.screenshot("before-dismiss");

    await dismiss.tap();
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
    session = await switchMobileTask(session, otherTask.title);
    session = await switchMobileTask(session, task.title);
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
    const laterCard = session
      .activeChat()
      .getByTestId("session-recovery-action-message")
      .filter({ hasText: "Git push failed again" });
    await expect(laterCard).toBeVisible();
    await laterCard.getByText("Technical details", { exact: true }).tap();
    await expect(laterCard.getByText(DIAGNOSTIC, { exact: true })).toBeVisible();
    await expect(laterCard.getByTestId("git-fix-button")).toBeVisible();
    await expect(laterCard.getByTestId("git-push-error-dismiss-button")).toBeVisible();
  });
});
