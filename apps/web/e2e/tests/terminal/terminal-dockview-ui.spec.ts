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

  /**
   * Regression: closing a terminal tab via dockview's X button must
   * PARK the terminal (preserving the PTY + DB row), not destroy it.
   * After reload, the closed terminal must surface in the "+" → Terminals
   * menu with a `parked` pill AND its original seq, so the user can
   * resume it.
   *
   * Before the fix this fails because dockview-layout-setup.onDidRemovePanel
   * calls stopUserShell (alias for destroyUserShell) — the DB row is
   * gone, no "parked" row exists to find.
   */
  test("closing a terminal parks it and the row shows up after reload with parked pill", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await createTaskAndWait(apiClient, seedData, "Close + Reload UI");
    const session = await openTask(testPage, "Close + Reload UI");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    // Add a second terminal so we have a non-default one to close.
    await clickNewTerminalInPlusMenu(testPage, session);
    await expect(testPage.getByTestId("terminal-tab-seq-1")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toBeVisible({ timeout: 5_000 });

    // Close the seq=2 tab specifically. `.last()` is unreliable when
    // terminals span multiple groups (default lives in the right-bottom,
    // user-created lands in the center). Walk from the seq=2 badge up
    // to its wrapping div and grab that subtree's close button.
    const seq2Close = testPage
      .getByTestId("terminal-tab-seq-2")
      .locator("..")
      .locator(".dv-default-tab-action");
    await seq2Close.click();
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toHaveCount(0, { timeout: 5_000 });

    // Layout-save is 300ms-debounced. Wait past the debounce so the
    // saved JSON reflects the closed/parked terminal — otherwise the
    // saved layout still has it and the reload re-opens it.
    await testPage.waitForTimeout(800);

    // Reload — only the open default terminal should restore as a
    // panel; the parked one lives in the reopen menu.
    await testPage.reload();
    await session.waitForLoad();
    await session.clickTab("Terminal");

    // The open default terminal should still render as a visible tab.
    const terminalContent = testPage.locator(".dv-default-tab-content").filter({
      hasText: /^Terminal$/,
    });
    await expect
      .poll(() => terminalContent.count(), {
        timeout: 10_000,
        message: "default terminal tab should still be visible after reload",
      })
      .toBeGreaterThanOrEqual(1);

    // Open "+" menu — the parked second terminal must appear under
    // Terminals with its seq=2 marker AND the `parked` pill.
    await session.addPanelButton().click();
    const terminalSection = testPage
      .locator('[role="menu"]')
      .getByText("Terminals", { exact: true });
    await expect(terminalSection).toBeVisible({ timeout: 10_000 });

    // The seq=2 row must be present — proves the close was a park, not
    // a destroy. The row no longer carries a distinct "parked" pill;
    // its mere presence after a destroy-on-close handler would have
    // wiped the DB row is the regression guard.
    const reopenRowsWithSeq2 = testPage
      .locator('[data-testid^="reopen-terminal-"]')
      .filter({ hasText: "#2" });
    await expect
      .poll(() => reopenRowsWithSeq2.count(), {
        timeout: 10_000,
        message:
          "parked terminal #2 should appear in reopen menu — if missing, close-handler destroyed instead of parked",
      })
      .toBe(1);
  });

  /**
   * Regression: with two open terminals AND a page reload (no close),
   * both terminals must re-render with their badges. Before the fix the
   * serializer rewrites the user-created terminal's panel id to
   * `terminal-saved-N` and drops the environmentId/taskID params, so
   * after reload that panel has no shell entry in the store → no badge,
   * fallback title text.
   */
  test("two open terminals survive a reload with their seq badges intact", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await createTaskAndWait(apiClient, seedData, "Reload Badges UI");
    const session = await openTask(testPage, "Reload Badges UI");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    await clickNewTerminalInPlusMenu(testPage, session);
    await expect(testPage.getByTestId("terminal-tab-seq-1")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toBeVisible({ timeout: 5_000 });

    // Layout-save is 300ms-debounced. Wait past the debounce so the
    // saved JSON includes the user-created terminal panel before we
    // reload — otherwise a fast test races the save.
    await testPage.waitForTimeout(800);

    await testPage.reload();
    await session.waitForLoad();

    // After reload, both badges must reappear — proves both panels'
    // store entries (kind=ordinary, seq) were preserved across restore.
    await expect(testPage.getByTestId("terminal-tab-seq-1")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toBeVisible({ timeout: 5_000 });

    // No tab content should contain "Terminal N" text — seq belongs in
    // the badge, not the title.
    const numberedTitles = testPage.locator(".dv-default-tab-content").filter({
      hasText: /^Terminal\s+\d+$/,
    });
    expect(
      await numberedTitles.count(),
      'tab title should be plain "Terminal" after reload (seq belongs in the badge)',
    ).toBe(0);
  });

  /**
   * Regression: the row × destroy affordance in the "+" → Terminals
   * menu permanently deletes a terminal (PTY stopped, DB row removed,
   * no return after reload). Solves the discoverability gap once the
   * tab is closed and the right-click Destroy is no longer reachable.
   */
  test("destroy button on a reopen-menu row permanently removes the terminal", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await createTaskAndWait(apiClient, seedData, "Row Destroy UI");
    const session = await openTask(testPage, "Row Destroy UI");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    await clickNewTerminalInPlusMenu(testPage, session);
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toBeVisible({ timeout: 10_000 });

    // Park the seq=2 tab by closing it (last in tab order).
    const terminalCloseButtons = testPage
      .locator(".dv-default-tab")
      .filter({ hasText: /^Terminal/ })
      .locator(".dv-default-tab-action");
    await terminalCloseButtons.last().click();

    // Open the "+" menu and confirm-destroy the seq=2 row.
    await session.addPanelButton().click();
    const row = testPage.locator('[data-testid^="reopen-terminal-"]').filter({ hasText: "#2" });
    await expect(row).toHaveCount(1, { timeout: 10_000 });

    testPage.once("dialog", (d) => d.accept());
    await row.getByTestId("destroy-terminal-row").click();

    // Row vanishes from the live menu.
    await expect(row).toHaveCount(0, { timeout: 5_000 });

    // Reload and confirm it does not come back.
    // Close the open menu first by pressing Escape.
    await testPage.keyboard.press("Escape");
    await testPage.reload();
    await session.waitForLoad();
    await session.addPanelButton().click();
    const rowAfter = testPage
      .locator('[data-testid^="reopen-terminal-"]')
      .filter({ hasText: "#2" });
    await expect(rowAfter).toHaveCount(0, { timeout: 5_000 });
  });

  /**
   * Regression: clicking an existing-open terminal row in the "+" menu
   * must focus the existing tab — NOT add a second panel for the same
   * PTY. Before the fix, the default-migrated panel kept its id as
   * `terminal-default` while the reopen row carried the shell-<uuid>,
   * so api.getPanel(uuid) missed and a duplicate tab was created.
   */
  test("reopen-menu row for an already-open terminal focuses, does not duplicate", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await createTaskAndWait(apiClient, seedData, "Focus Existing UI");
    const session = await openTask(testPage, "Focus Existing UI");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    // Create a second terminal so we have a non-default row to click.
    await clickNewTerminalInPlusMenu(testPage, session);
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toBeVisible({ timeout: 10_000 });
    await testPage.waitForTimeout(800);

    // Count terminal tab content elements before the focus click.
    const terminalContent = testPage.locator(".dv-default-tab-content").filter({
      hasText: /^Terminal$/,
    });
    const before = await terminalContent.count();
    expect(before, "two terminal tabs before clicking reopen").toBeGreaterThanOrEqual(2);

    // Open the menu and click the seq=2 row — that terminal is already
    // open as a tab, so the click should focus rather than mint a new
    // panel.
    await session.addPanelButton().click();
    const seq2Row = testPage.locator('[data-testid^="reopen-terminal-"]').filter({ hasText: "#2" });
    await expect(seq2Row).toHaveCount(1, { timeout: 10_000 });
    await seq2Row.click();

    // Tab count must NOT grow. The menu closes on its own; wait briefly
    // for any spurious panel to land.
    await testPage.waitForTimeout(500);
    const after = await terminalContent.count();
    expect(
      after,
      `clicking an open terminal in the reopen menu must focus, not duplicate (before=${before}, after=${after})`,
    ).toBe(before);
  });

  /**
   * Inline rename: right-clicking a terminal tab and picking Rename
   * swaps the title in place for an editable input. Typing + Enter
   * commits, updating the tab title (custom names override the
   * default "Terminal").
   */
  test("right-click → Rename swaps the tab title for an inline input", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await createTaskAndWait(apiClient, seedData, "Inline Rename UI");
    const session = await openTask(testPage, "Inline Rename UI");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    // The default panel's ContextMenuTrigger has a testid prefixed
    // with `terminal-tab-shell-` once the migration to a DB-backed
    // shell-<uuid> id completes.
    const tabTrigger = testPage.locator('[data-testid^="terminal-tab-shell-"]').first();
    await expect(tabTrigger).toBeVisible({ timeout: 15_000 });
    await tabTrigger.click({ button: "right" });

    // Pick the Rename menu item.
    await testPage.getByRole("menuitem", { name: /^Rename/ }).click();

    // Input replaces the title text. Type a custom name and Enter.
    const input = testPage.getByTestId("terminal-tab-rename-input");
    await expect(input).toBeVisible({ timeout: 5_000 });
    await input.fill("build watcher");
    await input.press("Enter");

    // Tab title now reads "build watcher" — proves the rename committed
    // and the displayName lookup found the customName.
    await expect(
      testPage.locator(".dv-default-tab-content").filter({ hasText: "build watcher" }).first(),
    ).toBeVisible({ timeout: 5_000 });
  });
});
