import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

/**
 * UI-level E2E coverage for the dockview terminal experience. The
 * existing terminal-first-class.spec asserts WS RPC behaviour; this
 * spec asserts what the user actually SEES on the page.
 *
 * Each test runs against the real dockview layout (desktop project).
 */

async function createTaskAndWait(apiClient: ApiClient, seedData: SeedData, title: string) {
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

async function openTask(page: Page, title: string): Promise<SessionPage> {
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

async function clickNewTerminalInPlusMenu(page: Page, session: SessionPage) {
  await session.addPanelButton().click();
  await page.getByTestId("new-terminal-button").click();
}

test.describe("Terminals — dockview UI", () => {
  /**
   * Regression: the tab title for ordinary terminals should be the
   * literal "Terminal" (no " N" suffix) with the sequence number in a
   * sibling badge — matching the session-tab pattern where the agent
   * name is the title and the seq is a pill before it.
   *
   * Before the fix this test fails because `DockviewDefaultTab` reads
   * from `api.title` directly and ignores any prop overrides; the
   * panel was created with `title="Terminal 2"` so the tab text reads
   * "Terminal 2" with the badge also visible → "2 Terminal 2".
   */
  test("multi-terminal tabs show seq badge + plain 'Terminal' title", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await createTaskAndWait(apiClient, seedData, "Tab Badge UI");
    const session = await openTask(testPage, "Tab Badge UI");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    // Open the dockview "+" menu, then click the new "New Terminal"
    // row that lives under the Terminals section label.
    await clickNewTerminalInPlusMenu(testPage, session);

    // The strip now has two terminal panels. Each tab's visible text
    // should be exactly "Terminal" — the seq lives in the adjacent
    // badge, not in the title itself.
    const terminalTabs = testPage
      .locator(".dv-default-tab-content")
      .filter({ hasText: /^Terminal/ });
    await expect
      .poll(() => terminalTabs.count(), { timeout: 10_000, message: "two terminal tabs visible" })
      .toBeGreaterThanOrEqual(2);

    // None of the tab content nodes should contain "Terminal 1" or
    // "Terminal 2" — the seq must be in the badge sibling, not the
    // title.
    const numberedTitles = testPage.locator(".dv-default-tab-content").filter({
      hasText: /^Terminal\s+\d+$/,
    });
    expect(
      await numberedTitles.count(),
      'tab title should be plain "Terminal" (seq belongs in the badge)',
    ).toBe(0);

    // Both seq badges should be present and adjacent to a "Terminal"
    // title. The badges are rendered with `data-testid="terminal-tab-seq-N"`.
    await expect(testPage.getByTestId("terminal-tab-seq-1")).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toBeVisible({ timeout: 5_000 });
  });
});
