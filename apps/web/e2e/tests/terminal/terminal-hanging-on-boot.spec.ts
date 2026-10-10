import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Create a task with the seeded mock agent and wait for it to settle.
 * The mock agent processes a "/e2e:simple-message" prompt and finishes,
 * so the session reaches COMPLETED before we navigate. This is the same
 * pattern used in terminal-env-keyed.spec.ts.
 */
async function createTaskAndWaitForDone(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  executorProfileId?: string,
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
      ...(executorProfileId ? { executor_profile_id: executorProfileId } : {}),
    },
  );

  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 30_000, message: `Waiting for ${title} session to settle` },
    )
    .toBe(true);

  return task;
}

/** Open a task from the task listing and wait for the session view. */
async function navigateToTaskFromListing(page: Page, title: string): Promise<SessionPage> {
  const kanban = new KanbanPage(page);
  await kanban.goto();
  const session = new SessionPage(page);
  // New tasks appear in the sidebar before the Kanban board's filtered
  // columns necessarily include their cards.
  const sidebarTask = session.sidebarTaskItem(title);
  await expect(sidebarTask).toBeVisible({ timeout: 15_000 });
  await sidebarTask.click();
  await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });
  await session.waitForLoad();
  return session;
}

test.describe("Terminal hangs on Connecting", () => {
  /**
   * Reproduces: opening a task whose session has already finished sometimes
   * leaves the shell terminal panel stuck on "Connecting terminal..." forever.
   *
   * Why this hangs in current code: the shell terminal gates its WS open on
   * `agentctlStatus.isReady` AND a non-null `environmentId`. Both come from
   * events the backend only publishes when an execution is created. When the
   * user lands on the task page after the agent's `agentctl_ready` event has
   * already fired (and was missed because the WS wasn't connected yet), the
   * frontend stays in `starting` and never opens the WS — so the backend's
   * lazy `GetOrEnsureExecutionForEnvironment` is never invoked. Chicken-egg.
   *
   * Asserts the loading overlay disappears within a reasonable budget.
   */
  test("shell terminal connects on cold load of a finished task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const task = await createTaskAndWaitForDone(apiClient, seedData, "Cold Load Terminal Task");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();
  });

  /**
   * Reproduces: switching from one task to another from the task listing
   * leaves the second task's terminal stuck on "Connecting terminal...".
   *
   * Two tasks are created and both run to completion before any navigation
   * happens — guaranteeing the agentctl events fired with no client connected.
   * After landing on task B (the second navigation), the terminal panel must
   * connect within a reasonable budget instead of hanging.
   */
  test("shell terminal connects after switching tasks from the listing", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await createTaskAndWaitForDone(apiClient, seedData, "Switch Task Alpha");
    await createTaskAndWaitForDone(apiClient, seedData, "Switch Task Beta");

    const session = await navigateToTaskFromListing(testPage, "Switch Task Alpha");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    // Return to the listing and open Beta. Its sidebar row can be ready before
    // the filtered Kanban board publishes its card.
    const sessionB = await navigateToTaskFromListing(testPage, "Switch Task Beta");
    await sessionB.clickTab("Terminal");
    await sessionB.expectTerminalConnected();
  });

  /**
   * Reproduces: hard-reloading the page while viewing a task drops the
   * agentctl status the client had cached from the original launch events.
   * After reload the snapshot the backend sends on `session.subscribe`
   * doesn't replay agentctl status, so the gate stays stuck and the
   * terminal panel hangs on "Connecting terminal...".
   */
  test("shell terminal reconnects after hard reload", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(60_000);
    await createTaskAndWaitForDone(apiClient, seedData, "Reload Terminal Task");

    const session = await navigateToTaskFromListing(testPage, "Reload Terminal Task");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    await testPage.reload();
    const sessionAfter = new SessionPage(testPage);
    await sessionAfter.waitForLoad();
    await sessionAfter.clickTab("Terminal");
    await sessionAfter.expectTerminalConnected();
  });

  /**
   * Reproduces: switching between tasks via the in-task sidebar (no full
   * navigation) leaves the next task's terminal stuck on "Connecting
   * terminal...". Sidebar navigation uses SPA client-side routing so
   * the same React tree mounts a different task; the env+agentctl gate
   * has to recompute reactively. PR #755/#758 introduced new state
   * dependencies (`environmentIdBySessionId`, `useEnvironmentId`) that
   * occasionally read `null` for the just-clicked session.
   */
  test("shell terminal connects after switching tasks via sidebar", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await createTaskAndWaitForDone(apiClient, seedData, "Sidebar Switch Alpha");
    await createTaskAndWaitForDone(apiClient, seedData, "Sidebar Switch Beta");

    const session = await navigateToTaskFromListing(testPage, "Sidebar Switch Alpha");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    // In-task sidebar navigation: click the other task while staying on /t/.
    const beta = session.taskInSidebar("Sidebar Switch Beta");
    await expect(beta).toBeVisible({ timeout: 15_000 });
    await beta.click();
    // The Terminal tab persists across the switch (env-keyed dockview), so
    // the panel must reconnect for the new task's environment.
    await session.expectTerminalConnected();
  });

  /**
   * User-reported repro (PT): "quando vou para um local executor o terminal
   * nao aparece e depois fica lixado para todas as outras tasks, tenho de
   * mandar hard refresh".
   *
   * Translation: visiting a task that uses a local-executor profile leaves
   * its terminal stuck on Connecting AND poisons the terminal for every
   * other task visited afterwards — only a hard refresh recovers.
   *
   * Tasks A and C use the default mock agent profile. Task B uses the
   * worktree (local) executor profile. The flow:
   *   1. Open A → terminal connects.
   *   2. Switch to B (local executor) → its terminal must connect.
   *   3. Switch to C (default again) → terminal must STILL connect.
   *
   * If step 2 hangs, step 3 also hangs without a hard reload — this asserts
   * neither task's terminal gets stuck regardless of executor profile mix.
   */
  test("local-executor task switch does not poison terminal for other tasks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);

    await createTaskAndWaitForDone(apiClient, seedData, "Default Exec A");
    await createTaskAndWaitForDone(
      apiClient,
      seedData,
      "Local Exec B",
      seedData.worktreeExecutorProfileId,
    );
    await createTaskAndWaitForDone(apiClient, seedData, "Default Exec C");

    const session = await navigateToTaskFromListing(testPage, "Default Exec A");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    // Switch to local-executor task via sidebar (in-app nav, no full reload).
    const taskB = session.taskInSidebar("Local Exec B");
    await expect(taskB).toBeVisible({ timeout: 15_000 });
    await taskB.click();
    await session.expectTerminalConnected();

    // Switch to a different default-executor task — must not be poisoned by B.
    const taskC = session.taskInSidebar("Default Exec C");
    await expect(taskC).toBeVisible({ timeout: 15_000 });
    await taskC.click();
    await session.expectTerminalConnected();
  });
});
