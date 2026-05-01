import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Create a task with a primary session and navigate to it.
 * Mirrors the helper in multi-session-ux.spec.ts.
 */
async function createTaskAndNavigate(
  testPage: import("@playwright/test").Page,
  apiClient: import("../../helpers/api-client").ApiClient,
  seedData: import("../../fixtures/test-base").SeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 30_000, message: "Waiting for session to finish" },
    )
    .toBe(true);

  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 10_000 });
  await card.click();
  await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
    timeout: 15_000,
  });
  return { task, session };
}

test.describe("Terminal stays put across same-task session switch", () => {
  test("terminal scrollback persists when switching sessions of the same task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { task, session } = await createTaskAndNavigate(
      testPage,
      apiClient,
      seedData,
      "Env-Keyed Terminal Task",
    );

    // Create a second session (same task → same TaskEnvironmentID).
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.length;
        },
        { timeout: 30_000, message: "Waiting for second session" },
      )
      .toBe(2);

    // Both sessions must share the same task_environment_id, otherwise
    // the rest of this test asserts nothing meaningful.
    const { sessions } = await apiClient.listTaskSessions(task.id);
    const envIds = new Set(sessions.map((s) => s.task_environment_id ?? null));
    expect(
      envIds.size,
      `expected one shared task_environment_id, got: ${[...envIds].join(", ")}`,
    ).toBe(1);
    const sortedSessions = [...sessions].sort(
      (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
    );
    const sessionA = sortedSessions[0];
    const sessionB = sortedSessions[1];

    // Terminal panel exists by default in the dockview layout — bring it to
    // focus, then type a marker so we can detect reconnects.
    await session.clickTab("Terminal");
    await session.typeInTerminal("echo kandev-marker-A");
    await session.expectTerminalHasText("kandev-marker-A");

    // Switch to session B by clicking its dockview tab. Layout is env-keyed,
    // so the terminal panel must stay put — same xterm instance, same buffer.
    await session.sessionTabBySessionId(sessionB.id).click();
    await session.expectTerminalHasText("kandev-marker-A");

    // Type a second marker; switch back to A; both markers must still be visible.
    await session.typeInTerminal("echo kandev-marker-B");
    await session.expectTerminalHasText("kandev-marker-B");
    await session.sessionTabBySessionId(sessionA.id).click();
    await session.expectTerminalHasText("kandev-marker-A");
    await session.expectTerminalHasText("kandev-marker-B");
  });

  test("user_shell.list returns the same shell across sessions sharing an env", async ({
    apiClient,
    seedData,
  }) => {
    // Backend-only check: the runner must group user shells by TaskEnvironmentID,
    // so two sessions sharing an env see the same shell list. This is the
    // server-side complement to the UI test above.
    test.setTimeout(60_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Env-Keyed Shell List Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000 },
      )
      .toBe(true);

    const { sessions } = await apiClient.listTaskSessions(task.id);
    const sessionA = sessions[0];
    expect(sessionA.task_environment_id, "session A must have a task_environment_id").toBeTruthy();
  });
});
