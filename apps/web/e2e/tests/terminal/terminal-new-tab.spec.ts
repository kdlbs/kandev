import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * Create a task and wait for the seeded mock agent to settle. Mirrors
 * terminal-hanging-on-boot.spec.ts so the SSR-loaded base terminal is
 * always present before we test creating extras.
 */
async function createTaskAndWaitForDone(apiClient: ApiClient, seedData: SeedData, title: string) {
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
      { timeout: 30_000, message: `Waiting for ${title} session to settle` },
    )
    .toBe(true);

  return task;
}

async function navigateToTaskViaKanban(page: Page, title: string): Promise<SessionPage> {
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

/**
 * Captures outgoing JSON-RPC payloads for `user_shell.create` so the test can
 * assert env-keying — payload must include `task_environment_id` and must NOT
 * include `session_id`. This is the regression guard for the bug that broke
 * `+ → Terminal` on every session lacking an env mapping.
 */
function recordUserShellCreatePayloads(page: Page): {
  collected: () => Array<Record<string, unknown>>;
} {
  const captured: Array<Record<string, unknown>> = [];
  page.on("websocket", (ws) => {
    ws.on("framesent", (event) => {
      const raw =
        typeof event.payload === "string" ? event.payload : (event.payload?.toString("utf8") ?? "");
      if (!raw || !raw.includes('"user_shell.create"')) return;
      try {
        const parsed = JSON.parse(raw) as { action?: string; payload?: Record<string, unknown> };
        if (parsed.action === "user_shell.create") {
          captured.push(parsed.payload ?? {});
        }
      } catch {
        // non-JSON / binary — ignore
      }
    });
  });
  return { collected: () => captured };
}

test.describe("Terminal new tab — env-keyed user shell RPCs", () => {
  /**
   * Repro of the user-reported bug: clicking the right-panel "+" button to
   * add a second terminal silently failed because the WS RPC was keyed by
   * session_id and the session lacked a task_environment_id link.
   *
   * After the fix, the RPC carries task_environment_id directly and the heal
   * pass guarantees the env mapping exists.
   */
  test("right-panel '+' button adds a second terminal tab", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const recorder = recordUserShellCreatePayloads(testPage);

    await createTaskAndWaitForDone(apiClient, seedData, "New Tab Right Panel");
    const session = await navigateToTaskViaKanban(testPage, "New Tab Right Panel");

    // Base terminal must be connected before we add tabs.
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    // Right-panel SessionTabs uses `addButtonLabel="+"` → button text is "+".
    const addButton = testPage.locator(`button:has-text("+")`).first();
    await addButton.click();

    // The new tab is labeled "Terminal 2" by the backend (first plain shell
    // is "Terminal" and not closable; subsequent ones are "Terminal N").
    const newTab = testPage.locator(`button[role="tab"]:has-text("Terminal 2")`);
    await expect(newTab).toBeVisible({ timeout: 15_000 });

    // Regression guard: the WS payload must be env-keyed.
    await expect.poll(() => recorder.collected().length, { timeout: 5_000 }).toBeGreaterThan(0);
    for (const payload of recorder.collected()) {
      expect(payload).toHaveProperty("task_environment_id");
      expect(payload).not.toHaveProperty("session_id");
      expect(payload.task_environment_id).toMatch(/.+/); // non-empty
    }
  });

  /**
   * The dockview group "+" dropdown also creates a new terminal panel.
   * Different code path (dockview-header-actions.tsx → addTerminalPanel)
   * but same env-keyed RPC underneath.
   */
  test("dockview '+' menu adds a Terminal panel", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(60_000);
    const recorder = recordUserShellCreatePayloads(testPage);

    await createTaskAndWaitForDone(apiClient, seedData, "New Tab Dockview Menu");
    const session = await navigateToTaskViaKanban(testPage, "New Tab Dockview Menu");

    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    // Open the dockview "+" menu (every group has one — the chat/center group's
    // is reliably present and not behind any conditional).
    await session.addPanelButton().click();
    const terminalItem = testPage.getByRole("menuitem", { name: "Terminal" });
    await expect(terminalItem).toBeVisible({ timeout: 5_000 });
    await terminalItem.click();

    // Two terminal panels are now in the dockview tree.
    await expect
      .poll(() => testPage.locator(".dv-default-tab:has-text('Terminal')").count(), {
        timeout: 10_000,
      })
      .toBeGreaterThanOrEqual(2);

    // Regression guard for env-keyed RPC.
    await expect.poll(() => recorder.collected().length, { timeout: 5_000 }).toBeGreaterThan(0);
    for (const payload of recorder.collected()) {
      expect(payload).toHaveProperty("task_environment_id");
      expect(payload).not.toHaveProperty("session_id");
    }
  });

  /**
   * Heal coverage: a session created without task_environment_id (legacy /
   * orphan rows) must still get terminals working after the boot-time heal
   * runs. We can't easily trigger a backend restart from the e2e harness, so
   * we assert the heal runs at server startup by checking that every session
   * the API returns has task_environment_id populated. The unit tests in
   * apps/backend/internal/task/repository/sqlite/heal_test.go cover the SQL.
   */
  test("listed sessions all have task_environment_id (post-heal)", async ({
    apiClient,
    seedData,
  }) => {
    const task = await createTaskAndWaitForDone(apiClient, seedData, "Heal Coverage Task");
    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions.length).toBeGreaterThan(0);
    for (const s of sessions) {
      expect(s.task_environment_id, `session ${s.id} missing env id`).toMatch(/.+/);
    }
  });
});
