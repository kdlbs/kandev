import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Create a passthrough (TUI) agent profile for the mock agent. */
async function createTUIProfile(apiClient: ApiClient, name: string) {
  const { agents } = await apiClient.listAgents();
  return apiClient.createAgentProfile(agents[0].id, name, {
    model: "mock-fast",
    auto_approve: true,
    cli_passthrough: true,
  });
}

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

// ---------------------------------------------------------------------------
// Tests — ACP (normal) mode
// ---------------------------------------------------------------------------

test.describe("Session resume (ACP mode)", () => {
  // These tests restart the backend mid-test, which can be flaky
  test.describe.configure({ retries: 1 });

  test("resume after backend restart preserves messages and accepts new prompts", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // 1. Create task and start agent with a simple scenario
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Resume ACP Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Navigate to the session and wait for agent to finish its first turn
    const session = await openTaskSession(testPage, "Resume ACP Task");
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    // 3. Verify the "Started agent Mock" boot message appeared on initial launch
    await expect(session.chat.getByText("Started agent Mock", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Restart the backend — kills the process, respawns with same DB/config
    await backend.restart();

    // 5. Reload the page so SSR fetches from the new backend instance
    await testPage.reload();
    await session.waitForLoad();

    // 6. Previous messages should still be visible (loaded from DB via SSR)
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });
    await expect(session.chat.getByText("/e2e:simple-message")).toBeVisible({ timeout: 15_000 });

    // 7. Wait for auto-resume to complete — useSessionResumption hook detects
    //    needs_resume=true and relaunches the agent via session.launch.
    //    The full cycle (backend restart → health check → page reload → SSR →
    //    WS reconnect → auto-resume → agent turn) can be slow under CI load.
    await expect(session.idleInput()).toBeVisible({ timeout: 60_000 });

    // 8. Verify the "Resumed agent Mock" boot message appeared after resume
    await expect(session.chat.getByText("Resumed agent Mock", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 9. Send a follow-up message to verify the agent works after resume
    await session.sendMessage("/e2e:simple-message");

    // 10. The agent should respond to the new prompt
    await expect(
      session.chat.getByText("simple mock response", { exact: false }).nth(1),
    ).toBeVisible({ timeout: 30_000 });
  });
});

// ---------------------------------------------------------------------------
// Tests — TUI (passthrough) mode
// ---------------------------------------------------------------------------

test.describe("Session resume (TUI passthrough mode)", () => {
  test.describe.configure({ retries: 1 });

  test("resume TUI session after backend restart reconnects with resume flag", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);

    // 1. Create a TUI agent profile
    const tuiProfile = await createTUIProfile(apiClient, "TUI Resume");

    // 2. Create task with TUI agent
    await apiClient.createTaskWithAgent(seedData.workspaceId, "TUI Resume Task", tuiProfile.id, {
      description: "hello from resume test",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    // 3. Navigate and wait for TUI terminal to load
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("TUI Resume Task");
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForPassthroughLoad();
    await session.waitForPassthroughLoaded();

    // 4. Verify initial TUI header
    await session.expectPassthroughHasText("Mock Agent");

    // 5. Wait for the workflow step to advance (idle timeout fires turn complete)
    await expect(session.stepperStep("Review")).toHaveAttribute("aria-current", "step", {
      timeout: 30_000,
    });

    // 6. Restart the backend
    await backend.restart();

    // 7. Reload the page — forces SSR re-fetch and WS reconnect
    await testPage.reload();

    // 8. Wait for passthrough terminal to reconnect after resume
    await session.waitForPassthroughLoad();
    await session.waitForPassthroughLoaded();

    // 9. The TUI should show the RESUMED header, confirming --resume/-c was passed
    await session.expectPassthroughHasText("RESUMED", 30_000);
  });
});
