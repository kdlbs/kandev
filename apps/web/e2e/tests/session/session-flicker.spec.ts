import { expect } from "@playwright/test";
import type { Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

/**
 * Regression: opening / switching to a second agent session in a task that
 * already has one open must NOT make the UI flicker uncontrollably between the
 * old and new session.
 *
 * Root cause of the original bug: every session of a task is supposed to share
 * one `task_environment_id` (the backend reuses the task's environment), but a
 * launch race can hand a second session a *different* env id. Dockview layouts
 * are keyed by env, so a same-task env change triggered a full env-layout
 * switch that strips the sibling session's chat panel (keepSessionId). With the
 * active session bouncing between the two envs, the tabs were repeatedly
 * removed and re-added — the "flicker between the old and new session" users
 * reported. Closing one tab stopped it; reopening it restarted it.
 *
 * These tests assert the active session settles and, critically, that two
 * same-task sessions whose env ids differ keep BOTH tabs and never tear the
 * chat panel down.
 */

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

type E2EStore = {
  getState: () => {
    tasks: { activeSessionId: string | null };
  };
  setState: (updater: (s: { environmentIdBySessionId: Record<string, string> }) => void) => void;
  subscribe: (cb: () => void) => () => void;
};

type DockApi = {
  panels: Array<{ id: string }>;
  onDidRemovePanel: (cb: (p: { id: string }) => void) => void;
};

type FlickerWindow = {
  __KANDEV_E2E_STORE__?: E2EStore;
  __dockviewApi__?: DockApi;
  __activeLog__?: Array<string | null>;
  __panelRemovals__?: string[];
  __divergeIntervalId__?: number;
};

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

/** Longest run of strict A,B,A,B alternation in a value sequence. */
function longestAlternationRun(values: Array<string | null>): number {
  let best = 0;
  let run = 0;
  for (let i = 1; i < values.length; i++) {
    if (values[i] !== values[i - 1] && (i < 2 || values[i] === values[i - 2])) {
      run = run === 0 ? 2 : run + 1;
    } else {
      run = values[i] !== values[i - 1] ? 2 : 0;
    }
    best = Math.max(best, run);
  }
  return best;
}

test.describe("Session flicker", () => {
  test("opening a second session does not flicker the active session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { session, session1Id, session2Id } = await createTaskWithTwoSessions(
      testPage,
      apiClient,
      seedData,
      "Flicker Second Session",
    );

    await session.sessionTabBySessionId(session1Id).click();
    await testPage.waitForTimeout(300);

    // Observe every change of the active session over the settle window.
    await testPage.evaluate(() => {
      const w = window as unknown as FlickerWindow;
      const store = w.__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge missing");
      const log: Array<string | null> = [];
      w.__activeLog__ = log;
      let last = store.getState().tasks.activeSessionId;
      log.push(last);
      store.subscribe(() => {
        const v = store.getState().tasks.activeSessionId;
        if (v !== last) {
          last = v;
          log.push(v);
        }
      });
    });

    await session.sessionTabBySessionId(session2Id).click();
    await testPage.waitForTimeout(2_500);

    const activeLog = await testPage.evaluate(
      () => (window as unknown as FlickerWindow).__activeLog__ ?? [],
    );

    // One deliberate switch (A -> B), no sustained oscillation.
    expect(longestAlternationRun(activeLog)).toBeLessThan(4);
    expect(activeLog.length).toBeLessThan(5);
    await expect(session.sessionTabBySessionId(session1Id)).toBeVisible();
    await expect(session.sessionTabBySessionId(session2Id)).toBeVisible();
  });

  test("two same-task sessions with diverged env ids keep both tabs and the chat", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const { session, session1Id, session2Id } = await createTaskWithTwoSessions(
      testPage,
      apiClient,
      seedData,
      "Flicker Diverged Env",
    );

    await session.sessionTabBySessionId(session1Id).click();
    await testPage.waitForTimeout(300);

    // Simulate the backend launch race: session #2 ends up with a DIFFERENT
    // task_environment_id than session #1. Keep re-forcing it so trailing WS
    // env-sync events can't reset it during the test window. Also record any
    // session chat-panel teardown.
    await testPage.evaluate((sid2) => {
      const w = window as unknown as FlickerWindow;
      const store = w.__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge missing");
      // Fail fast if the dockview bridge is absent — otherwise the removal
      // listener below would never register and the teardown assertion would
      // pass trivially (false green).
      const dockviewApi = w.__dockviewApi__;
      if (!dockviewApi) throw new Error("Dockview API bridge missing");
      w.__divergeIntervalId__ = window.setInterval(() => {
        store.setState((s) => {
          s.environmentIdBySessionId[sid2] = "diverged-env-for-session-2";
        });
      }, 30);
      const removals: string[] = [];
      w.__panelRemovals__ = removals;
      dockviewApi.onDidRemovePanel((p: { id: string }) => {
        if (p.id.startsWith("session:")) removals.push(p.id);
      });
    }, session2Id);
    await testPage.waitForTimeout(100);

    // Switch to session #2 (now on a diverged env) and back — this is what used
    // to strip the sibling tab and tear the chat down.
    await session.sessionTabBySessionId(session2Id).click();
    await testPage.waitForTimeout(800);
    await session.sessionTabBySessionId(session1Id).click();
    await testPage.waitForTimeout(800);
    await session.sessionTabBySessionId(session2Id).click();
    await testPage.waitForTimeout(1_500);

    // Both tabs must remain present throughout, and the active session's chat
    // must still be visible (dockview keeps inactive panels mounted-but-hidden,
    // so assert on a *visible* session-chat rather than the first match).
    await expect(session.sessionTabBySessionId(session1Id)).toBeVisible();
    await expect(session.sessionTabBySessionId(session2Id)).toBeVisible();
    await expect(testPage.locator('[data-testid="session-chat"]:visible').first()).toBeVisible();

    // Both session chat panels must still be mounted, and the layout must not
    // have torn any of them down as part of a (spurious) env switch — that
    // teardown is the flicker.
    const panels = await testPage.evaluate(() => {
      const api = (window as unknown as { __dockviewApi__?: { panels: Array<{ id: string }> } })
        .__dockviewApi__;
      return (api?.panels ?? []).map((p) => p.id).filter((id) => id.startsWith("session:"));
    });
    expect(panels).toContain(`session:${session1Id}`);
    expect(panels).toContain(`session:${session2Id}`);

    const removals = await testPage.evaluate(() => {
      const w = window as unknown as FlickerWindow;
      // Stop forcing the diverged env so the interval can't outlive the test.
      if (w.__divergeIntervalId__ !== undefined) {
        clearInterval(w.__divergeIntervalId__);
        w.__divergeIntervalId__ = undefined;
      }
      return w.__panelRemovals__ ?? [];
    });
    expect(removals).toEqual([]);
  });
});
