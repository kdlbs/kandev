import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const TERMINAL_STATES = ["COMPLETED", "FAILED", "CANCELLED"];

/**
 * An ended session without workspace restoration must explain why its shell
 * is unavailable. Successful workspace-only restoration may connect a shell
 * while leaving the agent session stopped.
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

/** Model an ended environment whose backend does not admit automatic recovery. */
async function preventWorkspaceRestore(page: Page, sessionId: string) {
  let statusReceived = false;
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    server.onMessage((message) => {
      if (typeof message !== "string") {
        socket.send(message);
        return;
      }
      socket.send(
        message
          .split("\n")
          .map((part) => {
            if (!part.trim()) return part;
            const frame = JSON.parse(part);
            if (
              frame.type !== "response" ||
              frame.action !== "task.session.status" ||
              frame.payload?.session_id !== sessionId
            )
              return part;
            statusReceived = true;
            return JSON.stringify({
              ...frame,
              payload: {
                ...frame.payload,
                auto_resume_allowed: false,
                needs_workspace_restore: false,
              },
            });
          })
          .join("\n"),
      );
    });
  });
  return () => statusReceived;
}

test.describe("terminal on an ended session", () => {
  // testPage is the fixture that points baseURL at the worker's own frontend.
  for (const restore of [false, true]) {
    test(
      restore
        ? "connects a restored workspace without restarting the stopped agent"
        : "shows the ended-session reason instead of a connecting spinner",
      async ({ testPage, apiClient, seedData }) => {
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

        const endedState = await state();
        const statusReceived = restore ? null : await preventWorkspaceRestore(testPage, sessionId!);
        await testPage.goto(`/t/${task.id}`);
        await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`));

        const session = new SessionPage(testPage);
        await session.waitForLoad();

        const terminalPanel = testPage.getByTestId("terminal-panel").first();
        await expect(terminalPanel).toBeVisible({ timeout: 15_000 });

        if (restore) {
          await expect
            .poll(() =>
              testPage.evaluate((id) => {
                const state = window.__KANDEV_E2E_STORE__!.getState();
                const environment = state.environmentIdBySessionId[id];
                return state.workspaceRestoration.byEnvironmentId[environment]?.status;
              }, sessionId!),
            )
            .toBe("ready");
          await expect
            .poll(() =>
              terminalPanel.locator(".xterm").evaluate((element) => {
                const host = element.parentElement as HTMLElement & {
                  __xtermReadBuffer?: () => string;
                };
                return host.__xtermReadBuffer?.() ?? "";
              }),
            )
            .not.toBe("");
          await session.typeInTerminal("printf 'RESTORED_STOPPED_%s\\n' SHELL");
          await session.expectTerminalHasText("RESTORED_STOPPED_SHELL");
          await expect(terminalPanel.getByTestId("passthrough-session-ended")).toBeHidden();
          expect(await state()).toBe(endedState);
          const status = await apiClient.wsRequest<{ is_agent_running: boolean }>(
            "task.session.status",
            { task_id: task.id, session_id: sessionId },
          );
          expect(status.is_agent_running).toBe(false);
        } else {
          await expect.poll(statusReceived!).toBe(true);
          // The assertion that matters: the reason is on screen.
          await expect(terminalPanel.getByTestId("passthrough-session-ended")).toBeVisible({
            timeout: 15_000,
          });
        }
        // And the lying spinner is not. Without this the test would still pass
        // while an opaque overlay covered the message.
        await expect(terminalPanel.getByTestId("passthrough-loading")).toBeHidden();
      },
    );
  }
});
