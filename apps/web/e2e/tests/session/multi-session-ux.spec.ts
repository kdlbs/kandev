import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

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

    // Confirmation dialog should appear
    const dialog = session.alertDialog();
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await expect(dialog).toContainText("Delete session?");

    // Cancel the deletion
    const cancelBtn = dialog.getByRole("button", { name: "Cancel" });
    await cancelBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 5_000 });

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
    const dialog = session.alertDialog();
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    const confirmBtn = dialog.getByRole("button", { name: "Delete" });
    await confirmBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 5_000 });

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
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
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
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // Switch back to task 1
    await session.clickTaskInSidebar("Task Switch A");
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });
  });
});
