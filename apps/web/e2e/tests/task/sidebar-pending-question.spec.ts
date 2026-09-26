/**
 * Regression test for kdlbs/kandev#1657: the sidebar "waiting for input"
 * question icon must show for a task blocked on an agent clarification even
 * when the task has never been opened, purely from the task snapshot.
 */
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { activeSessionId, seedSecondaryClarificationTask } from "../../helpers/clarification";
import { waitForSessionDone, waitForSessionState } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

type ClarificationMessage = {
  type?: string;
  content?: string;
  metadata?: Record<string, unknown>;
};

function pendingID(message: ClarificationMessage): string | null {
  if (message.type !== "clarification_request") return null;
  const value = message.metadata?.pending_id;
  return typeof value === "string" ? value : null;
}

async function clarificationMessages(apiClient: ApiClient, sessionId: string) {
  return (await apiClient.listSessionMessages(sessionId)).messages as ClarificationMessage[];
}

async function waitForSessionWaitingForInput(
  apiClient: ApiClient,
  taskId: string,
  message: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        return sessions[0]?.state ?? "";
      },
      { message, timeout: 60_000 },
    )
    .toBe("WAITING_FOR_INPUT");
}

async function waitForSessionTurnsComplete(
  apiClient: ApiClient,
  sessionId: string,
  message: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { turns } = await apiClient.listSessionTurns(sessionId);
        return turns.length > 0 && turns.every((turn) => Boolean(turn.completed_at));
      },
      { message, timeout: 60_000 },
    )
    .toBe(true);
}

test.describe("Sidebar pending-question indicator without opening the task", () => {
  test("older detached clarification stays superseded after a newer bundle is skipped", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(180_000);
    const title = "Superseded clarification task";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      seedData.agentProfileId,
      {
        description: "/e2e:clarification-timeout",
        repository_ids: [seedData.repositoryId],
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    if (!task.session_id) throw new Error("supersession setup has no session");

    let olderPendingID: string | null = null;
    await expect
      .poll(
        async () => {
          const messages = await clarificationMessages(apiClient, task.session_id!);
          const detached = messages.find(
            (message) =>
              pendingID(message) &&
              message.metadata?.status === "pending" &&
              message.metadata?.agent_disconnected === true,
          );
          olderPendingID = detached ? pendingID(detached) : null;
          return olderPendingID;
        },
        {
          message: "timed-out clarification should become detached",
          timeout: 60_000,
        },
      )
      .not.toBeNull();
    await expect
      .poll(
        async () => {
          const messages = await clarificationMessages(apiClient, task.session_id!);
          return messages.some((message) => message.content?.includes("Question timed out"));
        },
        {
          message: "timed-out agent turn should persist its final response",
          timeout: 60_000,
        },
      )
      .toBe(true);
    await waitForSessionTurnsComplete(
      apiClient,
      task.session_id,
      "timed-out agent turn should finish before the next prompt",
    );
    await waitForSessionWaitingForInput(
      apiClient,
      task.id,
      "completed timeout turn should leave the session waiting for input",
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.clarificationDeferredNotice()).toBeVisible({ timeout: 30_000 });

    await apiClient.addUserMessage(task.id, task.session_id, "/e2e:clarification");
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "newer clarification should wait for input",
      timeout: 60_000,
    });

    let newerPendingID: string | null = null;
    await expect
      .poll(async () => {
        const ids = (await clarificationMessages(apiClient, task.session_id!))
          .map(pendingID)
          .filter((id): id is string => id !== null);
        newerPendingID = ids.find((id) => id !== olderPendingID) ?? null;
        return new Set(ids).size;
      })
      .toBe(2);
    expect(newerPendingID).not.toBe(olderPendingID);

    await expect(session.clarificationOverlay()).toBeVisible();
    await session.clarificationSkip().click();
    await expect
      .poll(async () => {
        const newer = (await clarificationMessages(apiClient, task.session_id!)).find(
          (message) => pendingID(message) === newerPendingID,
        );
        return newer?.metadata?.status;
      })
      .toBe("rejected");
    await waitForSessionTurnsComplete(
      apiClient,
      task.session_id,
      "skipped clarification turn should finish before the next prompt",
    );

    await apiClient.addUserMessage(
      task.id,
      task.session_id,
      'e2e:message("post-skip turn completed")',
    );
    await expect
      .poll(async () => {
        const { messages } = await apiClient.listSessionMessages(task.session_id!);
        return messages.some((message) => message.content?.includes("post-skip turn completed"));
      })
      .toBe(true);
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "post-skip turn should complete before reload",
      60_000,
    );

    await testPage.reload();
    await session.waitForLoad();
    await expect(
      session.activeChat().getByText("post-skip turn completed", { exact: true }),
    ).toBeVisible();
    const row = session.sidebarTaskItem(title);
    await expect(row).toBeVisible();
    await expect(
      row.locator(
        '[data-testid="task-state-turn-finished"], [data-testid="task-state-workflow-complete"]',
      ),
    ).toBeVisible();
    await expect(session.clarificationOverlay()).toHaveCount(0);
    await expect(row.getByTestId("task-state-waiting-for-input")).toHaveCount(0);
  });

  test("task activation opens the secondary session that owns clarification", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(180_000);
    const target = await seedSecondaryClarificationTask(
      apiClient,
      seedData,
      "Secondary clarification owner",
    );
    const navTask = await apiClient.createTask(seedData.workspaceId, "Secondary owner nav task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await testPage.goto(`/t/${navTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const targetRow = session.sidebarTaskItem(target.title);
    await expect(targetRow).toBeVisible();
    await expect(targetRow.getByTestId("task-state-waiting-for-input")).toBeVisible();
    await targetRow.click();

    await expect(testPage).toHaveURL(new RegExp(`/t/${target.id}$`));
    await expect.poll(() => activeSessionId(testPage)).toBe(target.clarificationSessionId);
    await expect(session.clarificationOverlay()).toBeVisible();
    await expect(session.clarificationOverlay()).toContainText(
      "Which database should we use for this project?",
    );
  });

  test("blocked task shows the question icon on a fresh page load; idle task does not", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    const blockedTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Blocked On Question",
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        repository_ids: [seedData.repositoryId],
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );

    const idleTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Finished Quietly",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        repository_ids: [seedData.repositoryId],
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );

    await waitForSessionWaitingForInput(
      apiClient,
      blockedTask.id,
      "blocked task session should park on the clarification",
    );
    await waitForSessionWaitingForInput(
      apiClient,
      idleTask.id,
      "idle task session should finish its turn",
    );

    const navTask = await apiClient.createTask(seedData.workspaceId, "Nav Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${navTask.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    const blockedRow = session.sidebarTaskItem("Blocked On Question");
    await expect(blockedRow).toBeVisible({ timeout: 10_000 });
    await expect(blockedRow.getByTestId("task-state-waiting-for-input")).toBeVisible({
      timeout: 10_000,
    });

    const idleRow = session.sidebarTaskItem("Finished Quietly");
    await expect(idleRow).toBeVisible({ timeout: 10_000 });
    await expect(idleRow.getByTestId("task-state-waiting-for-input")).toHaveCount(0);
    await expect(idleRow.getByTestId("task-state-pending-permission")).toHaveCount(0);
  });
});
