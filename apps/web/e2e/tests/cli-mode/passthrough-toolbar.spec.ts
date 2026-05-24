import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";

/**
 * CLI-mode parity: PassthroughToolbar mounts above the PTY in passthrough
 * sessions and exposes the kandev compose box + Stop button that ACP sessions
 * get via ChatInputArea. These tests verify:
 *
 *  1. The toolbar renders for passthrough sessions.
 *  2. The Chat toggle + send path forwards the typed message to the agent's
 *     stdin (mock-agent echoes it as "Processed: <text>").
 *  3. Stop sends Ctrl-C, causing the TUI process to exit and the session to
 *     leave the RUNNING / STARTING state.
 */
test.describe("CLI mode: passthrough toolbar", () => {
  /** Create a passthrough agent profile and return its ID. */
  async function createPassthroughProfile(apiClient: ApiClient, name: string): Promise<string> {
    const { agents } = await apiClient.listAgents();
    if (agents.length === 0) throw new Error("no agents registered in this e2e profile");
    const profile = await apiClient.createAgentProfile(agents[0].id, name, {
      model: "mock-fast",
      auto_approve: true,
      cli_passthrough: true,
    });
    return profile.id;
  }

  /**
   * Navigate to the kanban, click the task card, and wait for the passthrough
   * terminal to finish loading.
   */
  async function openTaskAndWaitForTerminal(
    testPage: import("@playwright/test").Page,
    kanban: KanbanPage,
    session: SessionPage,
    taskTitle: string,
  ) {
    const card = kanban.taskCardByTitle(taskTitle);
    await expect(card).toBeVisible({ timeout: 20_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    await session.waitForPassthroughLoad(20_000);
    await session.waitForPassthroughLoaded(20_000);
  }

  test("toolbar renders for passthrough sessions on the task page", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar Render");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Toolbar Render Task", profileId, {
      description: "hello toolbar",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar Render Task");

    await expect(testPage.getByTestId("passthrough-toolbar")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("passthrough-toggle-composer")).toBeVisible({
      timeout: 5_000,
    });
  });

  test("composer toggle + send forwards message to the agent", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar Send");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Toolbar Send Task", profileId, {
      description: "initial prompt",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar Send Task");

    // Wait for the initial prompt injection to complete
    await session.expectPassthroughHasText("Processed:", 20_000);

    // Open the composer and send a follow-up message
    await testPage.getByTestId("passthrough-toggle-composer").click();
    await expect(testPage.getByTestId("passthrough-composer")).toBeVisible({ timeout: 5_000 });

    const textarea = testPage.getByTestId("passthrough-composer-textarea");
    await textarea.fill("hello from e2e");
    await textarea.press("Enter");

    // The composer closes on successful send
    await expect(testPage.getByTestId("passthrough-composer")).toBeHidden({ timeout: 10_000 });

    // The mock-agent TUI echoes the prompt as "Processed: <text>"
    await session.expectPassthroughHasText("hello from e2e", 15_000);
  });

  test("Stop button sends Ctrl-C and the session transitions out of RUNNING", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar Stop");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Toolbar Stop Task", profileId, {
      // Use /sleep to keep the agent busy long enough for Stop to fire
      description: "/sleep 60",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar Stop Task");

    // Wait for the agent to reach a running state (Stop button becomes enabled)
    const stopBtn = testPage.getByTestId("passthrough-stop");
    await expect(stopBtn).toBeEnabled({ timeout: 20_000 });

    // Click Stop — sends \x03 to PTY stdin via agent.cancel WS handler
    await stopBtn.click();

    // The session should leave RUNNING/STARTING; the Stop button goes disabled
    await expect(stopBtn).toBeDisabled({ timeout: 20_000 });
  });
});
