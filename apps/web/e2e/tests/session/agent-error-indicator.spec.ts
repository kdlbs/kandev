import { test, expect } from "../../fixtures/test-base";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

const ERROR_MESSAGE = "peer disconnected before response";

test.describe("Task agent error indicator", () => {
  test("shows the last recoverable agent error in the sidebar and chat", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskTitle = `Recoverable Agent Error ${Date.now()}`;
    // Stamp the error in the future so the seeded agent messages from
    // /e2e:simple-message (created_at = now) are NOT considered "after" the
    // error — otherwise the sidebar icon auto-hides before we can assert it.
    const ERROR_OCCURRED_AT = new Date(Date.now() + 60 * 60 * 1000).toISOString();
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      taskTitle,
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
    await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for seeded session");

    await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      sessionId: task.session_id,
      agentProfileId: seedData.agentProfileId,
      metadata: {
        last_agent_error: {
          message: ERROR_MESSAGE,
          occurred_at: ERROR_OCCURRED_AT,
          agent_execution_id: "agent-exec-e2e",
        },
      },
    });
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const session = sessions.find((item) => item.id === task.session_id);
        const error = session?.metadata?.last_agent_error as { message?: string } | undefined;
        return error?.message ?? "";
      })
      .toBe(ERROR_MESSAGE);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // .first() - desktop and mobile sidebars can both be mounted in this layout.
    const taskRow = session.sidebarTaskItem(taskTitle).first();
    const errorIcon = taskRow.getByTestId("task-agent-error-icon");
    await expect(errorIcon).toBeVisible({ timeout: 15_000 });

    const notice = testPage.getByTestId("last-agent-error-notice");
    await expect(notice).toBeVisible({ timeout: 15_000 });
    await expect(notice).toContainText("Previous agent error");
    await expect(notice).toContainText(ERROR_MESSAGE);

    await errorIcon.hover();
    await expect(testPage.getByRole("tooltip")).toContainText(ERROR_MESSAGE);

    // Dismissing the chat notice should also clear the sidebar icon. Both
    // surfaces share the dismissal state through the UI store and the server
    // persists the dismissal for other browsers.
    await notice.getByRole("button", { name: "Hide previous agent error" }).click();
    await expect(notice).toBeHidden();
    await expect(errorIcon).toBeHidden();

    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const sessionMeta = sessions.find((item) => item.id === task.session_id)?.metadata;
        const error = sessionMeta?.last_agent_error as { dismissed_at?: string } | undefined;
        return error?.dismissed_at ?? "";
      })
      .not.toBe("");

    await testPage.evaluate(() => window.localStorage.removeItem("kandev.dismissedAgentErrors"));
    await testPage.reload();
    await session.waitForLoad();
    await expect(testPage.getByTestId("last-agent-error-notice")).toBeHidden();
    await expect(
      session.sidebarTaskItem(taskTitle).first().getByTestId("task-agent-error-icon"),
    ).toBeHidden();
  });
});
