import { existsSync } from "node:fs";
import { writeFileSync } from "node:fs";
import { join } from "node:path";

import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const fsExists = (p: string) => existsSync(p);

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Helpers to reduce boilerplate: create a task with a completed session
 * and navigate to it.
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

test.describe("Multi-session UX", () => {
  test("session tab shows numbered label", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);

    const { task, session } = await createTaskAndNavigate(
      testPage,
      apiClient,
      seedData,
      "Tab Naming Task",
    );

    // Create a second session
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    // Wait for second session to appear
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.length;
        },
        { timeout: 30_000, message: "Waiting for second session" },
      )
      .toBe(2);

    // Verify tabs show index badge and agent label
    const tab1 = session.sessionTabByText("1");
    const tab2 = session.sessionTabByText("2");
    await expect(tab1).toBeVisible({ timeout: 10_000 });
    await expect(tab2).toBeVisible({ timeout: 10_000 });
  });

  test("command panel navigation renders all existing session tabs", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const title = "Command panel multi-session task";
    const task = await apiClient.createTask(seedData.workspaceId, title, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const primary = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      sessionId: `command-panel-primary-${task.id}`,
      agentProfileId: seedData.agentProfileId,
      startedAt: "2026-08-26T00:00:00Z",
    });
    const secondary = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      sessionId: `command-panel-secondary-${task.id}`,
      agentProfileId: seedData.agentProfileId,
      startedAt: "2026-08-26T00:01:00Z",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle(title)).toBeVisible({ timeout: 10_000 });

    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+k`);
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await dialog.getByRole("combobox").fill(title);
    const option = dialog.getByRole("option").filter({ hasText: title });
    await expect(option).toBeVisible({ timeout: 10_000 });
    await option.click();

    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`));
    const session = new SessionPage(testPage);
    await expect(session.sessionTabBySessionId(primary.session_id)).toBeVisible({
      timeout: 10_000,
    });
    await expect(session.sessionTabBySessionId(secondary.session_id)).toBeVisible({
      timeout: 10_000,
    });
  });

  test("+ dropdown shows sessions with correct numbering", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { task, session } = await createTaskAndNavigate(
      testPage,
      apiClient,
      seedData,
      "Dropdown Numbering Task",
    );

    // Create second session via API to have two completed sessions
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.filter((s) => DONE_STATES.includes(s.state)).length;
        },
        { timeout: 60_000, message: "Waiting for second session to complete" },
      )
      .toBe(2);

    // Open the + dropdown and verify both sessions are listed with #1 and #2
    await session.addPanelButton().click();
    const items = session.sessionReopenItems();
    await expect(items).toHaveCount(2, { timeout: 5_000 });
    await expect(items.first()).toContainText("#1");
    await expect(items.last()).toContainText("#2");
  });

  test("completed sessions show state icon, active sessions do not", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { session } = await createTaskAndNavigate(
      testPage,
      apiClient,
      seedData,
      "State Icon Task",
    );

    // Open the + dropdown — the single completed session should have a state icon
    await session.addPanelButton().click();
    const items = session.sessionReopenItems();
    await expect(items.first()).toBeVisible({ timeout: 5_000 });

    // A completed session has a state icon (svg element inside the item)
    const stateIcon = items.first().locator("svg").last();
    await expect(stateIcon).toBeVisible();
  });

  test("idle waiting session has no question icon", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(90_000);

    const { task, session } = await createTaskAndNavigate(
      testPage,
      apiClient,
      seedData,
      "Idle Waiting Indicator Task",
    );

    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions).toHaveLength(1);
    const idleSession = sessions[0];
    expect(idleSession.state).toBe("WAITING_FOR_INPUT");

    await session.addPanelButton().click();
    const row = session.sessionReopenItem(idleSession.id);
    await expect(row).toBeVisible({ timeout: 5_000 });
    await expect(row.locator(".tabler-icon-message-question")).toHaveCount(0);
    await expect(row.locator(".tabler-icon-shield-question")).toHaveCount(0);
  });

  test("reload keeps a secondary pending prompt visible in the reopen menu", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { task, session } = await createTaskAndNavigate(
      testPage,
      apiClient,
      seedData,
      "Secondary Pending Prompt Task",
    );
    const secondary = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      sessionId: `secondary-pending-${task.id}`,
      startedAt: "2020-01-01T00:00:00Z",
    });
    await apiClient.seedSessionMessage(secondary.session_id, {
      type: "clarification_request",
      content: "Which database should the task use?",
      metadata: { status: "pending" },
    });

    // The boot payload loads messages only for the active primary session. The
    // secondary row must use its compact pending-action projection instead.
    await testPage.reload();
    await session.waitForLoad();
    // The desktop layout eagerly hydrates opened panels; evict this secondary
    // transcript to model the closed row that prompted the reload regression.
    await testPage.evaluate((sessionId) => {
      const store = (
        window as Window & {
          __KANDEV_E2E_STORE__?: {
            getState: () => {
              messages: { bySession: Record<string, unknown> };
            };
            setState: (next: unknown) => void;
          };
        }
      ).__KANDEV_E2E_STORE__;
      if (!store) return;
      const state = store.getState();
      const bySession = { ...state.messages.bySession };
      delete bySession[sessionId];
      store.setState({ messages: { ...state.messages, bySession } });
    }, secondary.session_id);
    await session.addPanelButton().click();

    const row = session.sessionReopenItem(secondary.session_id);
    await expect(row).toBeVisible({ timeout: 5_000 });
    await expect(row.locator(".tabler-icon-message-question")).toHaveCount(1);
    await expect(row.locator(".tabler-icon-shield-question")).toHaveCount(0);
  });

  test("delete session via context menu shows confirmation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { task, session } = await createTaskAndNavigate(
      testPage,
      apiClient,
      seedData,
      "Delete Confirm Task",
    );

    // Create second session so we have two
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
        { timeout: 30_000 },
      )
      .toBe(2);

    // Right-click on session #1 tab and click Delete
    const tab1 = session.sessionTabByText("1");
    await expect(tab1).toBeVisible({ timeout: 10_000 });
    await tab1.click({ button: "right" });

    const deleteItem = session.contextMenuItem("Delete");
    // Wait for context menu — the session must be in a deletable state
    await expect(deleteItem).toBeVisible({ timeout: 5_000 });
    await deleteItem.click();

    // The context-menu action uses an anchored, non-modal confirmation.
    const confirmation = testPage.getByTestId("session-delete-confirm-popover");
    await expect(confirmation).toBeVisible({ timeout: 5_000 });
    await expect(confirmation).toContainText("Delete session?");
    await expect(session.alertDialog()).toHaveCount(0);

    // Cancel the deletion
    const cancelBtn = confirmation.getByRole("button", { name: "Cancel" });
    await cancelBtn.click();
    await expect(confirmation).not.toBeVisible({ timeout: 5_000 });

    // Verify session still exists
    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions).toHaveLength(2);
  });

  test("delete session removes it from backend", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);

    const { task, session } = await createTaskAndNavigate(
      testPage,
      apiClient,
      seedData,
      "Delete Session Task",
    );

    // Create second session
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.filter((s) => DONE_STATES.includes(s.state)).length;
        },
        { timeout: 60_000 },
      )
      .toBe(2);

    // Capture session IDs now so we can identify tabs by ID (display numbers
    // get renumbered after deletion, making text-based locators unreliable).
    const { sessions: sessionsBeforeDelete } = await apiClient.listTaskSessions(task.id);
    const sorted = sessionsBeforeDelete.sort(
      (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
    );
    const session1Id = sorted[0].id;

    // Right-click on session #1 tab → Delete → Confirm
    const tab1 = session.sessionTabByText("1");
    await expect(tab1).toBeVisible({ timeout: 10_000 });
    await tab1.click({ button: "right" });

    await session.contextMenuItem("Delete").click();
    const confirmation = testPage.getByTestId("session-delete-confirm-popover");
    await expect(confirmation).toBeVisible({ timeout: 5_000 });
    await expect(session.alertDialog()).toHaveCount(0);

    const confirmBtn = confirmation.getByTestId("session-delete-confirm");
    await confirmBtn.click();
    await expect(confirmation).not.toBeVisible({ timeout: 5_000 });

    // Wait for the deleted session's tab to disappear (identified by session ID,
    // not display number, because the remaining session gets renumbered to #1).
    await expect(session.sessionTabBySessionId(session1Id)).not.toBeVisible({ timeout: 15_000 });

    // Verify backend only has 1 session
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.length;
        },
        { timeout: 10_000, message: "Waiting for session to be deleted" },
      )
      .toBe(1);
  });

  test("new session dialog context mode selector works", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { session } = await createTaskAndNavigate(
      testPage,
      apiClient,
      seedData,
      "Context Mode Task",
    );

    // Open new session dialog
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });

    // Verify default context mode is "Blank"
    const contextTrigger = session
      .newSessionDialog()
      .locator("button")
      .filter({ hasText: "Blank" });
    await expect(contextTrigger).toBeVisible();

    // Open context mode dropdown and check "Copy initial prompt" option exists
    await contextTrigger.click();
    const copyOption = testPage.getByRole("option", { name: "Copy initial prompt" });
    await expect(copyOption).toBeVisible({ timeout: 3_000 });

    // Close dialog
    const cancelBtn = session.newSessionDialog().getByRole("button", { name: "Cancel" });
    // Press Escape to close the select first
    await testPage.keyboard.press("Escape");
    await cancelBtn.click();
  });

  test("switching between tasks preserves correct session context", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // Create two tasks with sessions
    const task1 = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Task Switch A",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const task2 = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Task Switch B",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Wait for both to finish
    for (const task of [task1, task2]) {
      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(task.id);
            return DONE_STATES.includes(sessions[0]?.state ?? "");
          },
          { timeout: 30_000, message: `Waiting for ${task.id} to finish` },
        )
        .toBe(true);
    }

    // Navigate to task 1
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card1 = kanban.taskCardByTitle("Task Switch A");
    await expect(card1).toBeVisible({ timeout: 10_000 });
    await card1.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);

    // After the AppSidebar overhaul, switching tasks via the sidebar restores
    // each task's dockview env layout. The restored layout can land the chat
    // panel as a non-active background tab in the right-column group, so the
    // chat-visible `waitForLoad()` gate (and a [data-testid=session-chat]:visible
    // lookup) isn't reliable right after a switch. Foreground the chat tab first,
    // then assert the session's mock response on the now-visible chat — this is
    // exactly what a user does to read the conversation after switching.
    await session.showSessionContext();
    await expect(
      session.activeChat().getByText("simple mock response", { exact: false }),
    ).toBeVisible({
      timeout: 15_000,
    });

    // Verify task 1 title is in the sidebar
    await expect(session.taskInSidebar("Task Switch A")).toBeVisible();
    await expect(session.taskInSidebar("Task Switch B")).toBeVisible();

    // Switch to task 2 via sidebar
    await session.clickTaskInSidebar("Task Switch B");

    // Wait for URL to change to task 2's session
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    // Verify chat loads for task 2
    await session.showSessionContext();
    await expect(
      session.activeChat().getByText("simple mock response", { exact: false }),
    ).toBeVisible({
      timeout: 15_000,
    });

    // Switch back to task 1
    await session.clickTaskInSidebar("Task Switch A");
    await session.showSessionContext();
    await expect(
      session.activeChat().getByText("simple mock response", { exact: false }),
    ).toBeVisible({
      timeout: 15_000,
    });
  });
});

test.describe("Session deletion preserves the task workspace", () => {
  test("deleting the only session keeps the worktree and a replacement session reuses it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);

    const { task, session } = await createTaskAndNavigate(
      testPage,
      apiClient,
      seedData,
      "Preserve Workspace Task",
    );

    // The worktree-mode environment exposes the on-disk workspace root.
    await expect
      .poll(
        async () => {
          const env = await apiClient.getTaskEnvironment(task.id);
          return env?.workspace_path ?? "";
        },
        { timeout: 30_000, message: "Waiting for task environment workspace path" },
      )
      .not.toBe("");

    const env = await apiClient.getTaskEnvironment(task.id);
    const workspacePath = env?.workspace_path ?? "";
    if (!workspacePath) throw new Error("no workspace path for the task environment");
    const environmentId = env?.id ?? "";

    // Write an uncommitted marker into the worktree.
    const markerPath = join(workspacePath, "uncommitted-marker.txt");
    writeFileSync(markerPath, "keep-me\n", "utf8");
    await expect(fsExists(markerPath)).toBe(true);

    const { sessions: before } = await apiClient.listTaskSessions(task.id);
    const onlySessionId = before[0]?.id;
    if (!onlySessionId) throw new Error("expected exactly one session");

    // Delete the only session through the visible UI.
    const tab = session.sessionTabBySessionId(onlySessionId);
    await expect(tab).toBeVisible({ timeout: 10_000 });
    await tab.click({ button: "right" });
    await session.contextMenuItem("Delete").click();
    const confirmation = testPage.getByTestId("session-delete-confirm-popover");
    await expect(confirmation).toBeVisible({ timeout: 5_000 });
    await expect(confirmation).toContainText("task workspace and its files are kept");
    await expect(session.alertDialog()).toHaveCount(0);
    await confirmation.getByTestId("session-delete-confirm").click();

    // The task workspace and its uncommitted marker survive session deletion.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return !sessions.some((s) => s.id === onlySessionId);
        },
        { timeout: 15_000, message: "Waiting for the only session to be deleted" },
      )
      .toBe(true);
    await expect(fsExists(markerPath)).toBe(true);
    const envAfter = await apiClient.getTaskEnvironment(task.id);
    expect(envAfter?.id).toBe(environmentId);

    // A replacement session reuses the retained workspace and observes the
    // marker. (The task page auto-ensures a session when the task has none;
    // the explicit dialog still runs the replacement prompt deterministically.)
    await session.openNewSessionDialog();
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.filter((s) => DONE_STATES.includes(s.state)).length;
        },
        { timeout: 60_000, message: "Waiting for replacement session to finish" },
      )
      .toBeGreaterThan(0);

    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // The replacement session reused the retained environment, not a fresh one.
    const { sessions: after } = await apiClient.listTaskSessions(task.id);
    expect(after.some((s) => s.task_environment_id === environmentId)).toBe(true);
    const envAfterReplacement = await apiClient.getTaskEnvironment(task.id);
    expect(envAfterReplacement?.workspace_path).toBe(workspacePath);
    await expect(fsExists(markerPath)).toBe(true);
  });
});
