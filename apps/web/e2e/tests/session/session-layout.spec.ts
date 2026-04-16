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

  test("agent tab stays pinned when center tab strip overflows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    await seedTaskWithSession(testPage, apiClient, seedData, "Pinned Tab Test");

    // The session tab (agent chat) lives in the center group. Our hook marks
    // the wrapping `.dv-tab` with `dv-pinned-tab`.
    const sessionDvTab = testPage
      .locator(".dv-tab.dv-pinned-tab:has([data-testid^='session-tab-'])")
      .first();
    await expect(sessionDvTab).toBeVisible({ timeout: 10_000 });

    // Sanity check: the sticky CSS rule is actually active.  This catches
    // regressions where the `dv-pinned-tab` class is present but the CSS
    // rule is removed, overridden, or the element isn't inside a scrollable
    // ancestor that engages sticky.
    const stickyPosition = await sessionDvTab.evaluate(
      (el) => getComputedStyle(el as HTMLElement).position,
    );
    expect(stickyPosition).toBe("sticky");

    // Scope to the center group's tabs container (the one that holds the
    // session tab).
    const tabsContainer = testPage
      .locator(
        ".dv-tabs-and-actions-container:has([data-testid^='session-tab-']) .dv-tabs-container.dv-horizontal",
      )
      .first();

    // Simulate a user opening many diff/file tabs by injecting placeholder
    // `.dv-tab` siblings directly into the tabs container.  Going through
    // the + menu and opening real panels would also work, but that takes
    // several UI round-trips per tab and introduces dropdown flakiness.
    // The feature under test is purely about CSS sticky positioning, which
    // depends only on the DOM layout of siblings inside the scrolling
    // container — so synthetic siblings exercise the same code path.
    await tabsContainer.evaluate((el) => {
      const container = el as HTMLElement;
      for (let i = 0; i < 12; i++) {
        const dummy = document.createElement("div");
        dummy.className = "dv-tab";
        dummy.setAttribute("data-e2e-dummy-tab", "true");
        dummy.textContent = `Diff [file_${i}.go]`;
        dummy.style.cssText =
          "padding: 0 12px; min-width: 140px; display: inline-flex; align-items: center; flex-shrink: 0;";
        container.appendChild(dummy);
      }
    });

    // Confirm the strip now overflows its visible width.
    await expect
      .poll(
        async () =>
          tabsContainer.evaluate(
            (el) => (el as HTMLElement).scrollWidth > (el as HTMLElement).clientWidth,
          ),
        { timeout: 5_000, message: "Expected the tab strip to overflow" },
      )
      .toBe(true);

    // Scroll the strip all the way to the right — without sticky, this would
    // push the session tab off-screen.
    await tabsContainer.evaluate((el) => {
      (el as HTMLElement).scrollLeft = (el as HTMLElement).scrollWidth;
    });

    // With sticky positioning, the session tab should remain glued to the
    // left edge of the tab strip, regardless of scrollLeft.
    await expect
      .poll(
        async () => {
          const sessionBox = await sessionDvTab.boundingBox();
          const containerBox = await tabsContainer.boundingBox();
          if (!sessionBox || !containerBox) return Number.POSITIVE_INFINITY;
          return Math.abs(sessionBox.x - containerBox.x);
        },
        {
          timeout: 5_000,
          message: "Expected the session tab to stay pinned at the tab strip's left edge",
        },
      )
      .toBeLessThan(5);
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
