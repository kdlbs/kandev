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
  // testPage, not page: it is the fixture that points baseURL at the worker's
  // own frontend. With the default page, KanbanPage.goto()'s relative "/" has
  // nothing to resolve against and Playwright rejects it as an invalid URL.
  test("shows the ended-session reason instead of a connecting spinner", async ({
    testPage,
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

    // Let the seeded run finish on its own first. session.stop needs a live
    // execution to cancel, so calling it straight away fails with "no
    // execution for session" — the session has not started one yet.
    const state = async () => {
      const { sessions: current } = await apiClient.listTaskSessions(task.id);
      return current[0]?.state ?? "";
    };
    await expect
      .poll(state, { timeout: 60_000, message: "Waiting for the session to settle" })
      .toMatch(new RegExp([...TERMINAL_STATES, "WAITING_FOR_INPUT", "IDLE"].join("|")));

    // If it settled somewhere still live, cancel it. By now an execution
    // exists, so the stop lands.
    if (!TERMINAL_STATES.includes(await state())) {
      await apiClient.stopSession({ session_id: sessionId as string, force: true });
      await expect
        .poll(state, { timeout: 30_000, message: "Waiting for the cancel to land" })
        .toMatch(new RegExp(TERMINAL_STATES.join("|")));
    }

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardByTitle(title);
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const terminalPanel = testPage.getByTestId("terminal-panel").first();
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
