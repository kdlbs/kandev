import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

/**
 * Seed a task + session via the API and navigate directly to the session page.
 * Waits for the mock agent to complete its turn (idle input visible).
 */
async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  description = "/e2e:simple-message",
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  return session;
}

const TERMINAL_MARKER = "KANDEV_E2E_MARKER_12345";
const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

test.describe("Session layout", () => {
  // The standalone executor can fail to allocate a port on cold start;
  // retry once so a transient backend hiccup doesn't fail the suite.
  test.describe.configure({ retries: 1 });

  test("maximize terminal hides other panels", async ({ testPage, apiClient, seedData }) => {
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Maximize Test");

    // Default layout: all panels visible
    await session.expectDefaultLayout();

    // Type a command in the terminal
    await session.typeInTerminal(`echo ${TERMINAL_MARKER}`);
    await session.expectTerminalHasText(TERMINAL_MARKER);

    // Maximize the terminal group
    await session.clickMaximize();

    // Only terminal and sidebar should be visible, with our output
    await session.expectMaximized();
    await session.expectTerminalHasText(TERMINAL_MARKER);
  });

  test("maximize survives page refresh", async ({ testPage, apiClient, seedData }) => {
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Refresh Test");

    // Type a command in the terminal, then maximize
    await session.typeInTerminal(`echo ${TERMINAL_MARKER}`);
    await session.expectTerminalHasText(TERMINAL_MARKER);
    await session.clickMaximize();
    await session.expectMaximized();

    // Refresh the page — maximize state is saved in sessionStorage
    await testPage.reload();

    // After refresh: terminal should still be maximized
    await expect(session.terminal).toBeVisible({ timeout: 15_000 });
    await session.expectMaximized();
    // Terminal reconnects to the same shell — our output should still be there
    await session.expectTerminalHasText(TERMINAL_MARKER);
  });

  test("task switching preserves maximize per session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // Create Task A, type a command, and maximize terminal
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Task A Maximize");
    await session.typeInTerminal(`echo ${TERMINAL_MARKER}`);
    await session.expectTerminalHasText(TERMINAL_MARKER);
    await session.clickMaximize();
    await session.expectMaximized();

    // Remember Task A's URL so we can navigate back after visiting Task B
    const taskAUrl = testPage.url();

    // Create Task B via API and navigate directly
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Task B Normal",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!taskB.session_id) throw new Error("Task B creation did not return session_id");
    await testPage.goto(`/t/${taskB.id}`);

    const sessionB = new SessionPage(testPage);
    await sessionB.waitForLoad();
    await expect(sessionB.idleInput()).toBeVisible({ timeout: 30_000 });

    // Task B should have default (non-maximized) layout
    await sessionB.expectDefaultLayout();

    // Navigate back to Task A's session URL — this triggers a full page load,
    // exercising the tryRestoreLayout path that restores maximize from sessionStorage.
    await testPage.goto(taskAUrl);
    await expect(session.terminal).toBeVisible({ timeout: 15_000 });

    // Task A should still be maximized with our output
    await session.expectMaximized();
    await session.expectTerminalHasText(TERMINAL_MARKER);
  });

  test("closing maximized panel exits maximize and restores layout", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedTaskWithSession(testPage, apiClient, seedData, "Close Max Test");

    // Type a command in the terminal, then maximize
    await session.typeInTerminal(`echo ${TERMINAL_MARKER}`);
    await session.expectTerminalHasText(TERMINAL_MARKER);
    await session.clickMaximize();
    await session.expectMaximized();

    // Close the terminal tab via dockview's built-in tab close button.
    const terminalTab = testPage.locator(".dv-tab:has(.dv-default-tab:has-text('Terminal'))");
    const closeBtn = terminalTab.locator(".dv-default-tab-action");
    await closeBtn.click();

    // Should exit maximize and restore default layout minus the closed terminal
    await expect(session.chat).toBeVisible({ timeout: 10_000 });
    await expect(session.files).toBeVisible({ timeout: 10_000 });
    await expect(session.sidebar).toBeVisible();
    // Terminal should be gone (it was closed)
    await expect(session.terminal).not.toBeVisible({ timeout: 5_000 });
  });
});

test.describe("Session tab cleanup", () => {
  test("single-session task shows only named agent tab without star or number", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Single Session Tab Task",
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
        { timeout: 30_000, message: "Waiting for session to finish" },
      )
      .toBe(true);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardByTitle("Single Session Tab Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // Should have exactly one session tab (the named agent tab)
    const sessionTabs = testPage.locator("[data-testid^='session-tab-']");
    await expect(sessionTabs).toHaveCount(1, { timeout: 10_000 });

    // The generic "Agent" permanent tab should NOT exist
    // (it uses permanentTab tabComponent which has no data-testid, identified by text)
    const permanentAgentTab = testPage.locator(
      ".dv-default-tab:not(:has([data-testid^='session-tab-'])):has-text('Agent')",
    );
    await expect(permanentAgentTab).toHaveCount(0);

    // The single session tab should NOT show a star icon
    const star = sessionTabs.first().locator(".tabler-icon-star");
    await expect(star).toHaveCount(0);

    // The single session tab should NOT show a number badge
    // (number badges are small spans with bg-foreground/10 containing a digit)
    const numberBadge = sessionTabs.first().locator("span.rounded");
    await expect(numberBadge).toHaveCount(0);
  });
});
