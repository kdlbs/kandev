import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

/** Navigate to a kanban card by title and open its session page. */
async function openTaskSession(page: Page, title: string): Promise<SessionPage> {
  const kanban = new KanbanPage(page);
  await kanban.goto();

  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 15_000 });
  await card.click();
  await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

test.describe("Executor not found after backend restart", () => {
  // Backend restart tests can be flaky
  test.describe.configure({ retries: 1 });

  test("agent starts successfully when sending new prompt after backend restart", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // 1. Create task and start agent with a simple scenario
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Executor Fix Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Navigate to session and wait for agent to finish its first turn
    const session = await openTaskSession(testPage, "Executor Fix Task");
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    // 3. Restart the backend — clears in-memory execution store,
    //    but DB still has the old AgentExecutionID
    await backend.restart();

    // 4. Reload the page so SSR fetches from the new backend instance
    await testPage.reload();
    await session.waitForLoad();

    // 5. Wait for auto-resume to complete (workspace restoration)
    await expect(session.idleInput()).toBeVisible({ timeout: 60_000 });

    // 6. Send a NEW prompt — this triggers LaunchPreparedSession which
    //    reads the stale AgentExecutionID from DB and calls
    //    startAgentOnExistingWorkspace. Without the fix, this fails with
    //    "execution not found" and the session goes to FAILED state.
    await session.sendMessage("/e2e:simple-message");

    // 7. The agent should respond successfully (not fail with executor not found)
    await expect(
      session.chat.getByText("simple mock response", { exact: false }).nth(1),
    ).toBeVisible({ timeout: 30_000 });
  });
});
