import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

/**
 * CLI-mode parity: when a task is created with a passthrough (CLI / PTY)
 * agent and a user-typed description, the backend delivers the prompt
 * into the running CLI. The mock TUI echoes a "Processed: <prompt>"
 * line back, so the rendered xterm buffer contains both the prompt
 * text and the response — proof the agent received it.
 */
test.describe("CLI mode: prompt injection into PTY", () => {
  test("creating a CLI-mode task lands the description in the terminal buffer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Use the canonical e2e helper pattern: the e2e backend registers only
    // the mock-agent, so agents[0] is the passthrough-capable target.
    // Mirrors `createTUIProfile` in `apps/web/e2e/tests/terminal/terminal-agent.spec.ts`.
    const { agents } = await apiClient.listAgents();
    if (agents.length === 0) {
      throw new Error("no agents registered in this e2e profile");
    }
    const tui = await apiClient.createAgentProfile(agents[0].id, "CLI Inject", {
      model: "mock-fast",
      auto_approve: true,
      cli_passthrough: true,
    });

    const description = "Refactor the cron handler";

    await apiClient.createTaskWithAgent(seedData.workspaceId, "CLI Inject Task", tui.id, {
      description,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("CLI Inject Task");
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);

    // Terminal panel becomes visible — passthrough terminal mounts for
    // CLI-mode sessions.
    await session.waitForPassthroughLoad(15_000);
    await session.waitForPassthroughLoaded(15_000);

    // Generous timeout: passthrough idle window is 2s + buffer flush.
    // The prompt and the mock-agent's "Processed:" response should both
    // appear in the xterm buffer.
    await session.expectPassthroughHasText(description, 15_000);
    await session.expectPassthroughHasText("Processed:", 15_000);
  });
});
