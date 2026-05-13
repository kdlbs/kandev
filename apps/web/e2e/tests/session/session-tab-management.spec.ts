import { expect } from "@playwright/test";
import type { Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Regression suite for tab session management bugs:
 *  - Bug A: kanban.update wiped primarySessionId, dropping the primary star.
 *  - Bug B: handleDelete fired removeTaskSession before removePanel, so
 *           useAutoSessionTab re-created the deleted session's panel.
 *  - Bug C: setupChatPanelSafetyNet recreated panels for sessions that no
 *           longer existed in the store.
 */

type SetupResult = {
  task: { id: string };
  session: SessionPage;
  session1Id: string;
  session2Id: string;
};

async function createTaskWithTwoSessions(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SetupResult> {
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

  // First session must reach a done state before we open the second.
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 30_000, message: "Waiting for first session to finish" },
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

  // Launch the second session via the new-session dialog.
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
      { timeout: 60_000, message: "Waiting for both sessions to finish" },
    )
    .toBe(2);

  const { sessions } = await apiClient.listTaskSessions(task.id);
  const sorted = sessions.sort(
    (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
  );
  return { task, session, session1Id: sorted[0].id, session2Id: sorted[1].id };
}

function starInTab(session: SessionPage, sessionId: string) {
  return session.sessionTabBySessionId(sessionId).locator(".tabler-icon-star").first();
}

test.describe("Session tab management — close behavior", () => {
  test("deleting a non-active session removes its tab and stays gone", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { task, session, session1Id, session2Id } = await createTaskWithTwoSessions(
      testPage,
      apiClient,
      seedData,
      "Tab Stays Gone",
    );

    // Make session #2 active so #1 is the non-active deletion target.
    await session.sessionTabBySessionId(session2Id).click();

    await session.sessionTabBySessionId(session1Id).click({ button: "right" });
    await session.contextMenuItem("Delete").click();
    const dialog = session.alertDialog();
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await dialog.getByRole("button", { name: "Delete" }).click();

    // Tab disappears…
    await expect(session.sessionTabBySessionId(session1Id)).not.toBeVisible({ timeout: 15_000 });

    // …and stays gone — useAutoSessionTab must not recreate it.
    await testPage.waitForTimeout(800);
    await expect(session.sessionTabBySessionId(session1Id)).not.toBeVisible();
    await expect(session.sessionTabBySessionId(session2Id)).toBeVisible();

    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions).toHaveLength(1);
  });

  test("deleting the active session switches focus to the remaining session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { task, session, session1Id, session2Id } = await createTaskWithTwoSessions(
      testPage,
      apiClient,
      seedData,
      "Delete Active Session",
    );

    // Make session #1 active so the deletion target IS the active one.
    await session.sessionTabBySessionId(session1Id).click();

    await session.sessionTabBySessionId(session1Id).click({ button: "right" });
    await session.contextMenuItem("Delete").click();
    await expect(session.alertDialog()).toBeVisible({ timeout: 5_000 });
    await session.alertDialog().getByRole("button", { name: "Delete" }).click();

    await expect(session.sessionTabBySessionId(session1Id)).not.toBeVisible({ timeout: 15_000 });
    await expect(session.sessionTabBySessionId(session2Id)).toBeVisible({ timeout: 5_000 });

    // URL must not have switched to a different task.
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}`));

    // Backend must reflect the deletion — exactly one session remains.
    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions.map((s) => s.id)).toEqual([session2Id]);
  });

  test("deleting then immediately switching tasks does not resurrect the tab", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);

    const {
      task: task1,
      session,
      session1Id,
      session2Id,
    } = await createTaskWithTwoSessions(testPage, apiClient, seedData, "Delete Then Switch A");

    // Second task to switch into mid-delete.
    const task2 = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Delete Then Switch B",
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
          const { sessions } = await apiClient.listTaskSessions(task2.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for task2 session" },
      )
      .toBe(true);

    await session.sessionTabBySessionId(session1Id).click({ button: "right" });
    await session.contextMenuItem("Delete").click();
    await expect(session.alertDialog()).toBeVisible({ timeout: 5_000 });
    await session.alertDialog().getByRole("button", { name: "Delete" }).click();

    // Wait for backend to confirm deletion (don't wait for tab to disappear).
    await expect
      .poll(async () => (await apiClient.listTaskSessions(task1.id)).sessions.length, {
        timeout: 10_000,
      })
      .toBe(1);

    // Switch to task2 then back to task1 — the deleted tab must not return.
    await session.clickTaskInSidebar("Delete Then Switch B");
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    await session.waitForLoad();

    await session.clickTaskInSidebar("Delete Then Switch A");
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    await session.waitForLoad();

    await expect(session.sessionTabBySessionId(session2Id)).toBeVisible({ timeout: 10_000 });
    await testPage.waitForTimeout(800);
    await expect(session.sessionTabBySessionId(session1Id)).not.toBeVisible();
  });

  test("session tabs from one task do not leak into another task's view", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);

    const {
      session,
      session1Id: sessionA1Id,
      session2Id: sessionA2Id,
    } = await createTaskWithTwoSessions(testPage, apiClient, seedData, "Tab Leak Source A");

    // Task B has only one session — never visited from the UI yet.
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Tab Leak Target B",
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
          const { sessions } = await apiClient.listTaskSessions(taskB.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for taskB session" },
      )
      .toBe(true);
    const { sessions: bSessions } = await apiClient.listTaskSessions(taskB.id);
    const sessionB1Id = bSessions[0].id;

    // Sanity: while on task A, both A sessions are visible.
    await expect(session.sessionTabBySessionId(sessionA1Id)).toBeVisible({ timeout: 10_000 });
    await expect(session.sessionTabBySessionId(sessionA2Id)).toBeVisible({ timeout: 5_000 });

    // Switch from A → B via the sidebar.
    await session.clickTaskInSidebar("Tab Leak Target B");
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskB.id}`), { timeout: 15_000 });
    await session.waitForLoad();

    // Task B's only session must be visible…
    await expect(session.sessionTabBySessionId(sessionB1Id)).toBeVisible({ timeout: 10_000 });

    // …and neither of task A's session tabs should have followed us in.
    await testPage.waitForTimeout(800);
    await expect(session.sessionTabBySessionId(sessionA1Id)).not.toBeVisible();
    await expect(session.sessionTabBySessionId(sessionA2Id)).not.toBeVisible();

    // Reload to make sure the persisted layout for task B doesn't bring them back either.
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.sessionTabBySessionId(sessionB1Id)).toBeVisible({ timeout: 10_000 });
    await expect(session.sessionTabBySessionId(sessionA1Id)).not.toBeVisible();
    await expect(session.sessionTabBySessionId(sessionA2Id)).not.toBeVisible();
  });
});

test.describe("Session tab management — primary session persistence", () => {
  test("primary star survives a kanban.update broadcast", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);

    const { task, session, session1Id, session2Id } = await createTaskWithTwoSessions(
      testPage,
      apiClient,
      seedData,
      "Primary Through Kanban Update",
    );

    // Session #1 is auto-primary; promote session #2 via the context menu so we
    // can verify the new primary survives the next kanban.update broadcast.
    await session.sessionTabBySessionId(session2Id).click({ button: "right" });
    await expect(session.contextMenuItem("Set as Primary")).toBeVisible({ timeout: 5_000 });
    await session.contextMenuItem("Set as Primary").click();

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.find((s) => s.id === session2Id)?.is_primary ?? false;
        },
        { timeout: 10_000, message: "Waiting for primary set in backend" },
      )
      .toBe(true);

    await expect(starInTab(session, session2Id)).toBeVisible({ timeout: 5_000 });
    await expect(starInTab(session, session1Id)).not.toBeVisible({ timeout: 5_000 });

    // Trigger kanban.update by moving the task to a non-start step.
    const otherStep = seedData.steps.find((s) => s.id !== seedData.startStepId);
    if (!otherStep) throw new Error("Workflow needs at least 2 steps to trigger kanban.update");
    await apiClient.moveTask(task.id, seedData.workflowId, otherStep.id);

    // Give the WS broadcast time to land.
    await testPage.waitForTimeout(500);

    // Star must still be on session #2 (would jump back to #1 before the kanban.ts fix).
    await expect(starInTab(session, session2Id)).toBeVisible({ timeout: 5_000 });
    await expect(starInTab(session, session1Id)).not.toBeVisible();
  });

  test("primary star survives switching tasks and returning", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);

    const {
      task: task1,
      session,
      session1Id,
      session2Id,
    } = await createTaskWithTwoSessions(testPage, apiClient, seedData, "Primary Round Trip A");

    const task2 = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Primary Round Trip B",
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
          const { sessions } = await apiClient.listTaskSessions(task2.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000 },
      )
      .toBe(true);

    // Session #1 is auto-primary; promote session #2 (matches the user's bug
    // scenario: setting the second session as primary).
    await session.sessionTabBySessionId(session2Id).click({ button: "right" });
    await session.contextMenuItem("Set as Primary").click();
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task1.id);
          return sessions.find((s) => s.id === session2Id)?.is_primary ?? false;
        },
        { timeout: 10_000 },
      )
      .toBe(true);
    await expect(starInTab(session, session2Id)).toBeVisible({ timeout: 5_000 });

    // A → B → A round trip.
    await session.clickTaskInSidebar("Primary Round Trip B");
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    await session.waitForLoad();

    await session.clickTaskInSidebar("Primary Round Trip A");
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    await session.waitForLoad();

    await expect(session.sessionTabBySessionId(session1Id)).toBeVisible({ timeout: 10_000 });
    await expect(session.sessionTabBySessionId(session2Id)).toBeVisible({ timeout: 5_000 });
    await expect(starInTab(session, session2Id)).toBeVisible({ timeout: 5_000 });
    await expect(starInTab(session, session1Id)).not.toBeVisible();
  });
});
