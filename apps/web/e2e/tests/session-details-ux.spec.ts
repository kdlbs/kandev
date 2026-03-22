import { type Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";

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

test.describe("Session details UX", () => {
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
