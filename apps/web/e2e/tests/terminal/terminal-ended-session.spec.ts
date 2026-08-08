import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

const TERMINAL_STATES = ["COMPLETED", "FAILED", "CANCELLED"];

/**
 * A shell terminal on an ended session can never connect: the environment
 * handler refuses it, identically, on every attempt.
 *
 * The pane's loading overlay is opaque and full-bleed, so before this was
 * handled the user saw "Connecting terminal…" forever — a spinner that was not
 * merely unhelpful but false, since nothing was connecting. Anything written
 * into the xterm underneath it was invisible.
 *
 * This asserts the outcome rather than the mechanism: the pane must say the
 * session ended, and must not be showing a connecting spinner. A unit test on
 * the reconnect loop can see neither.
 */
test.describe("terminal on an ended session", () => {
  test("shows the ended-session reason instead of a connecting spinner", async ({
    page,
    apiClient,
    seedData,
  }) => {
    const title = `ended-session-terminal-${Date.now()}`;
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

    const { sessions } = await apiClient.listTaskSessions(task.id);
    const sessionId = sessions[0]?.id;
    expect(sessionId, "task must have a session to end").toBeTruthy();

    // Drive it to a terminal state rather than waiting for one, so the test
    // does not depend on how the seeded agent happens to finish.
    await apiClient.stopSession({ session_id: sessionId as string, force: true });
    await expect
      .poll(
        async () => {
          const { sessions: current } = await apiClient.listTaskSessions(task.id);
          return current[0]?.state ?? "";
        },
        { timeout: 30_000, message: "Waiting for the session to reach a terminal state" },
      )
      .toMatch(new RegExp(TERMINAL_STATES.join("|")));

    const kanban = new KanbanPage(page);
    await kanban.goto();
    const card = kanban.taskCardByTitle(title);
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(page).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(page);
    await session.waitForLoad();

    const terminalPanel = page.getByTestId("terminal-panel").first();
    await expect(terminalPanel).toBeVisible({ timeout: 15_000 });

    // The assertion that matters: the reason is on screen.
    await expect(terminalPanel.getByTestId("passthrough-session-ended")).toBeVisible({
      timeout: 15_000,
    });

    // And the lying spinner is not. Without this the test would still pass
    // while an opaque overlay covered the message.
    await expect(terminalPanel.getByTestId("passthrough-loading")).toBeHidden();
  });
});
