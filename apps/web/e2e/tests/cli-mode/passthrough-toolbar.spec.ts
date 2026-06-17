import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import { seedAvailableCommands } from "../../helpers/session-store";

/**
 * CLI-mode parity: PassthroughToolbar mounts above the PTY in passthrough
 * sessions and exposes the kandev compose box + Stop button that ACP sessions
 * get via ChatInputArea. These tests verify:
 *
 *  1. The toolbar renders for passthrough sessions.
 *  2. The Chat toggle + send path forwards the typed message to the agent's
 *     stdin (mock-agent echoes it as "Processed: <text>").
 *  3. No dedicated Stop button — users cancel via Ctrl-C in the xterm terminal
 *     (the toolbar intentionally omits a duplicate control).
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

  function passthroughEditor(testPage: import("@playwright/test").Page) {
    return testPage.getByTestId("passthrough-composer").locator(".tiptap.ProseMirror");
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

    const editor = passthroughEditor(testPage);
    await editor.fill("hello from e2e");
    await testPage.getByTestId("submit-message-button").click();

    // The composer closes on successful send
    await expect(testPage.getByTestId("passthrough-composer")).toBeHidden({ timeout: 10_000 });

    // The mock-agent TUI echoes the prompt as "Processed: <text>"
    await session.expectPassthroughHasText("hello from e2e", 15_000);
  });

  test("composer keeps slash literal and does not show command suggestions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar Commands");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Toolbar Commands Task",
      profileId,
      {
        description: "initial prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected passthrough task to start a session");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar Commands Task");
    await session.expectPassthroughHasText("Processed:", 20_000);
    await seedAvailableCommands(testPage, task.session_id, [
      { name: "slow", description: "Run slowly" },
      { name: "error", description: "Trigger an error" },
    ]);

    await testPage.getByTestId("passthrough-toggle-composer").click();
    const editor = passthroughEditor(testPage);
    await expect(editor).toBeVisible({ timeout: 5_000 });

    await editor.fill("/s");

    await expect(testPage.getByRole("listbox", { name: "Command suggestions" })).toHaveCount(0);
    await expect(editor).toHaveText("/s");
  });

  test("toolbar omits a Stop button; cancel is via Ctrl-C in the terminal", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar No Stop");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Toolbar No Stop Task", profileId, {
      description: "initial prompt",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar No Stop Task");

    await expect(testPage.getByTestId("passthrough-toolbar")).toBeVisible({ timeout: 10_000 });
    // PassthroughToolbar deliberately has no Stop affordance — Ctrl-C in xterm is the path.
    await expect(testPage.getByTestId("passthrough-stop")).toHaveCount(0);

    await session.passthroughTerminal.locator(".xterm").click();
    await testPage.keyboard.press("Control+c");
    // Terminal stays mounted; we only assert the UI never offered a misleading Stop button.
    await expect(testPage.getByTestId("passthrough-terminal")).toBeVisible({ timeout: 5_000 });
  });
});
