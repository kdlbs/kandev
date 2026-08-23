import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";
import {
  snapshotPersistedLayouts,
  waitForPersistedLayoutChange,
} from "../../helpers/dockview-persistence";
import { dwell } from "../../helpers/causal-waits";
import { pauseNextTerminalDestroy } from "./terminal-close-pause";
import { readTerminalHostBuffer } from "./terminal-test-helpers";

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

async function openTabletTask(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto("/");
  await page.evaluate(() => {
    window.localStorage.setItem("task-right-panel-collapsed", "false");
  });
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(page.getByTestId("tablet-task-layout")).toBeVisible({ timeout: 15_000 });
  return session;
}

async function listTerminalIds(
  apiClient: ApiClient,
  taskId: string,
  environmentId: string,
): Promise<string[]> {
  const response = await apiClient.wsRequest<{
    shells?: Array<{ id?: string; terminal_id?: string }>;
  }>("user_shell.list", {
    task_id: taskId,
    task_environment_id: environmentId,
    include_parked: true,
  });
  return (response.shells ?? [])
    .map((shell) => shell.id ?? shell.terminal_id)
    .filter((id): id is string => Boolean(id));
}

async function settleBrowserFrames(page: Page) {
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
  );
}

async function clickNewTerminalInPlusMenu(page: Page, session: SessionPage) {
  await session.addPanelButton().click();
  const menu = page
    .locator('[data-slot="dropdown-menu-content"]')
    .filter({ has: page.getByTestId("new-terminal-button") });
  await expect(menu).toBeVisible();
  await menu.getByTestId("new-terminal-button").click();
  // Radix keeps the portal mounted for its 100 ms close animation. Returning
  // sooner lets a second trigger click land while the first menu is closing;
  // that click is swallowed and leaves the dropdown closed. Wait for the
  // captured portal to detach so callers can safely reopen the menu.
  await expect(menu).toHaveCount(0);
}

test.describe("Terminals — dockview UI", () => {
  test("right-panel terminal context menu supports inline rename and confirmed terminate", async ({
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await createTaskAndWait(apiClient, seedData, "Right Panel Context Menu");
    const { sessions } = await apiClient.listTaskSessions(task.id);
    const environmentId = sessions[0]?.task_environment_id;
    expect(environmentId).toBeTruthy();
    if (!environmentId) throw new Error("task session is missing an environment id");

    const first = await apiClient.wsRequest<{ terminal_id: string }>("user_shell.create", {
      task_id: task.id,
      task_environment_id: environmentId,
    });
    const second = await apiClient.wsRequest<{ terminal_id: string }>("user_shell.create", {
      task_id: task.id,
      task_environment_id: environmentId,
    });
    await openTabletTask(tabletTestPage, task.id);
    const firstTab = tabletTestPage.getByTestId(`terminal-tab-${first.terminal_id}`);
    const secondTab = tabletTestPage.getByTestId(`terminal-tab-${second.terminal_id}`);
    const firstRenameInput = tabletTestPage
      .getByTestId(`terminal-tab-${first.terminal_id}`)
      .locator("..")
      .getByTestId(`terminal-tab-${first.terminal_id}-content`)
      .getByTestId("terminal-tab-rename-input");
    const secondRenameInput = tabletTestPage
      .getByTestId(`terminal-tab-${second.terminal_id}`)
      .locator("..")
      .getByTestId(`terminal-tab-${second.terminal_id}-content`)
      .getByTestId("terminal-tab-rename-input");
    await expect(firstTab).toBeVisible({ timeout: 15_000 });
    await expect(secondTab).toBeVisible({ timeout: 15_000 });

    // Double-click keeps the existing inline rename entry point functional.
    await firstTab.dblclick();
    await expect(firstRenameInput).toBeVisible();
    await expect(firstRenameInput).toBeFocused();
    await firstRenameInput.fill("double click rename");
    await tabletTestPage.keyboard.press("Enter");
    await expect(firstTab).toContainText("double click rename", { timeout: 10_000 });

    // The close button still opens the same local confirmation and returns
    // focus to that control when cancelled.
    await firstTab.hover();
    const firstClose = tabletTestPage.getByTestId(`terminal-tab-close-${first.terminal_id}`);
    await firstClose.click();
    const closeConfirmation = tabletTestPage.getByTestId("terminal-close-confirm-popover");
    await expect(closeConfirmation).toBeVisible();
    await closeConfirmation.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(firstClose).toBeFocused();
    await expect(firstTab).toBeVisible();

    // Context-menu actions are separate and do not invoke browser-native
    // prompt() or destroy the shell before the anchored confirmation.
    let nativeDialogSeen = false;
    tabletTestPage.once("dialog", (dialog) => {
      nativeDialogSeen = true;
      void dialog.dismiss();
    });
    await secondTab.click({ button: "right" });
    await tabletTestPage.getByRole("menuitem", { name: /^Rename/ }).click();
    await expect(secondRenameInput).toBeVisible();
    await expect(secondRenameInput).toBeFocused();
    await secondRenameInput.fill("context menu rename");
    await tabletTestPage.keyboard.press("Enter");
    await expect(secondTab).toContainText("context menu rename", { timeout: 10_000 });

    await secondTab.click({ button: "right" });
    await tabletTestPage.getByRole("menuitem", { name: /^Terminate/ }).click();
    await expect(closeConfirmation).toBeVisible();
    expect(nativeDialogSeen).toBe(false);
    await expect(secondTab).toBeVisible();
    await settleBrowserFrames(tabletTestPage);
    expect(
      await listTerminalIds(apiClient, task.id, environmentId),
      "context-menu terminate must wait for confirmation",
    ).toContain(second.terminal_id);

    await closeConfirmation.getByRole("button", { name: "Close terminal", exact: true }).click();
    await expect(secondTab).toHaveCount(0, { timeout: 10_000 });
    await expect
      .poll(() => listTerminalIds(apiClient, task.id, environmentId), {
        message: "confirmed context-menu terminate should destroy the shell",
      })
      .not.toContain(second.terminal_id);
  });

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
   * Regression: confirming a terminal close via dockview's X button must
   * destroy the shell (PTY stopped, DB row removed), not park it.
   * After reload the closed terminal must NOT surface in the "+" menu.
   */
  test("closing a terminal destroys it and the row does not return after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await createTaskAndWait(apiClient, seedData, "Close + Reload UI");
    const session = await openTask(testPage, "Close + Reload UI");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    await clickNewTerminalInPlusMenu(testPage, session);
    await expect(testPage.getByTestId("terminal-tab-seq-1")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toBeVisible({ timeout: 5_000 });

    const seq2Close = testPage
      .getByTestId("terminal-tab-seq-2")
      .locator("..")
      .locator(".dv-default-tab-action");
    const layoutBeforeClose = await snapshotPersistedLayouts(testPage);
    await seq2Close.click();
    const confirmation = testPage.getByTestId("terminal-close-confirm-popover");
    await expect(confirmation).toBeVisible();
    await confirmation.getByRole("button", { name: "Close terminal", exact: true }).click();
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toHaveCount(0, { timeout: 5_000 });

    // Local removal is intentionally optimistic. Observe background teardown
    // before reloading so the reload cannot race the server-owned shell row.
    const { sessions } = await apiClient.listTaskSessions(task.id);
    const environmentId = sessions[0]?.task_environment_id;
    expect(environmentId).toBeTruthy();
    await expect
      .poll(
        async () => {
          const response = await apiClient.wsRequest<{ shells?: Array<{ seq?: number }> }>(
            "user_shell.list",
            {
              task_id: task.id,
              task_environment_id: environmentId,
              include_parked: true,
            },
          );
          return response.shells?.some((shell) => shell.seq === 2) ?? false;
        },
        { timeout: 10_000, message: "terminal teardown did not reach server state" },
      )
      .toBe(false);

    // The tab is gone from the DOM, but the reload below reads sessionStorage,
    // and that write is on a ~300ms debounce with no event. Wait for the write
    // rather than past it: if the close never reaches storage, that is the bug
    // this test exists to catch, and it should fail here rather than as a
    // confusing assertion after the reload.
    await waitForPersistedLayoutChange(testPage, layoutBeforeClose);

    await testPage.reload();
    await session.waitForLoad();
    await session.clickTab("Terminal");

    const terminalContent = testPage.locator(".dv-default-tab-content").filter({
      hasText: /^Terminal$/,
    });
    await expect
      .poll(() => terminalContent.count(), {
        timeout: 10_000,
        message: "default terminal tab should still be visible after reload",
      })
      .toBeGreaterThanOrEqual(1);

    await session.addPanelButton().click();
    const terminalSection = testPage
      .locator('[role="menu"]')
      .getByText("Terminals", { exact: true });
    await expect(terminalSection).toBeVisible({ timeout: 10_000 });

    const reopenRowsWithSeq2 = testPage
      .locator('[data-testid^="reopen-terminal-"]')
      .filter({ has: testPage.getByTestId("reopen-terminal-seq-2") });
    await expect(reopenRowsWithSeq2).toHaveCount(0, { timeout: 5_000 });
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

    const layoutBeforeNewTerminal = await snapshotPersistedLayouts(testPage);
    await clickNewTerminalInPlusMenu(testPage, session);
    await expect(testPage.getByTestId("terminal-tab-seq-1")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toBeVisible({ timeout: 5_000 });

    // Layout-save is 300ms-debounced and publishes nothing, so the saved JSON
    // has to be observed rather than waited out: the reload below reads it, and
    // a slow save on a loaded shard would make this fail claiming the panel was
    // not preserved.
    await waitForPersistedLayoutChange(testPage, layoutBeforeNewTerminal);

    await testPage.reload();
    // After the dockview "preserve restored active tab" change (commit
    // 597b35662), the Terminal tab the user activated above stays active
    // on refresh, so session-chat is in the background — foreground it
    // explicitly so the page-loaded wait succeeds.
    await session.showSessionContext();

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
    const task = await createTaskAndWait(apiClient, seedData, "Row Destroy UI");
    const session = await openTask(testPage, "Row Destroy UI");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    await clickNewTerminalInPlusMenu(testPage, session);
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toBeVisible({ timeout: 10_000 });

    // Destroy the still-open seq=2 row from the "+" menu without closing its tab first.
    await session.addPanelButton().click();
    const row = testPage
      .locator('[data-testid^="reopen-terminal-"]')
      .filter({ has: testPage.getByTestId("reopen-terminal-seq-2") });
    await expect(row).toHaveCount(1, { timeout: 10_000 });
    const rowTestId = await row.getAttribute("data-testid");
    const terminalId = rowTestId?.replace("reopen-terminal-", "");
    expect(terminalId).toBeTruthy();

    const [terminalRowBox, adjacentRowBox] = await Promise.all([
      row.boundingBox(),
      testPage.getByTestId("new-terminal-button").boundingBox(),
    ]);
    expect(terminalRowBox).not.toBeNull();
    expect(adjacentRowBox).not.toBeNull();
    expect(terminalRowBox!.height).toBeCloseTo(adjacentRowBox!.height, 1);

    let nativeDialogSeen = false;
    testPage.once("dialog", (dialog) => {
      nativeDialogSeen = true;
      void dialog.dismiss();
    });
    const destroyButton = row.getByTestId("destroy-terminal-row");
    await destroyButton.click();

    expect(nativeDialogSeen).toBe(false);
    await expect(testPage.getByTestId("terminal-menu-close-confirmation")).toHaveCount(0);
    const confirmation = testPage.getByTestId("terminal-close-confirm-popover");
    await expect(confirmation).toBeVisible();

    const [rowBox, buttonBox, confirmationBox] = await Promise.all([
      row.boundingBox(),
      destroyButton.boundingBox(),
      confirmation.boundingBox(),
    ]);
    expect(rowBox).not.toBeNull();
    expect(buttonBox).not.toBeNull();
    expect(confirmationBox).not.toBeNull();
    expect(
      Math.abs(confirmationBox!.x + confirmationBox!.width - (buttonBox!.x + buttonBox!.width)),
    ).toBeLessThanOrEqual(8);

    // Radix menu rows take focus on pointer movement. Crossing the owning
    // menu on the way to the popover must not dismiss its actions.
    await testPage.mouse.move(rowBox!.x + 8, rowBox!.y + rowBox!.height / 2, { steps: 3 });
    await confirmation.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(confirmation).toBeHidden();
    await expect(row).toHaveCount(1);

    await destroyButton.click();
    await expect(confirmation).toBeVisible();
    await confirmation.getByRole("button", { name: "Close terminal", exact: true }).click();

    // Row vanishes from the live menu.
    await expect(row).toHaveCount(0, { timeout: 5_000 });

    // Local removal is intentionally optimistic. Observe the background
    // teardown at its server-owned list boundary before reloading, otherwise
    // a fast reload can race the request and temporarily restore the shell.
    const { sessions } = await apiClient.listTaskSessions(task.id);
    const environmentId = sessions[0]?.task_environment_id;
    expect(environmentId).toBeTruthy();
    await expect
      .poll(
        async () => {
          const response = await apiClient.wsRequest<{ shells?: Array<{ id?: string }> }>(
            "user_shell.list",
            {
              task_id: task.id,
              task_environment_id: environmentId,
              include_parked: true,
            },
          );
          return response.shells?.some((shell) => shell.id === terminalId) ?? false;
        },
        { timeout: 10_000, message: "terminal teardown did not reach server state" },
      )
      .toBe(false);

    // Reload and confirm it does not come back.
    // Close the open menu first by pressing Escape.
    await testPage.keyboard.press("Escape");
    await testPage.reload();
    await session.waitForLoad();
    await session.addPanelButton().click();
    const rowAfter = testPage
      .locator('[data-testid^="reopen-terminal-"]')
      .filter({ has: testPage.getByTestId("reopen-terminal-seq-2") });
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

    // Count terminal tab content elements before the focus click. Polling the
    // count is the wait: it returns as soon as the second tab has rendered
    // instead of after a fixed budget, and it fails if the tab never arrives
    // rather than reporting a baseline of 1 that makes the comparison below
    // meaningless.
    const terminalContent = testPage.locator(".dv-default-tab-content").filter({
      hasText: /^Terminal$/,
    });
    await expect
      .poll(() => terminalContent.count(), {
        timeout: 10_000,
        message: "two terminal tabs before clicking reopen",
      })
      .toBeGreaterThanOrEqual(2);
    const before = await terminalContent.count();

    // Open the menu and click the seq=2 row — that terminal is already
    // open as a tab, so the click should focus rather than mint a new
    // panel.
    await session.addPanelButton().click();
    const seq2Row = testPage
      .locator('[data-testid^="reopen-terminal-"]')
      .filter({ has: testPage.getByTestId("reopen-terminal-seq-2") });
    await expect(seq2Row).toHaveCount(1, { timeout: 10_000 });
    await seq2Row.click();

    // Tab count must NOT grow. There is no event for a panel that must never
    // be created, so the only way to give a regression room to happen is to let
    // the window in which it would have landed elapse.
    await dwell(
      testPage,
      500,
      "negative-assertion",
      "asserts that clicking an already-open terminal focuses rather than minting a second panel; a panel that must not appear publishes nothing to wait on",
    );
    const after = await terminalContent.count();
    expect(
      after,
      `clicking an open terminal in the reopen menu must focus, not duplicate (before=${before}, after=${after})`,
    ).toBe(before);
  });

  test("tab close uses localized confirmation and removes before teardown settles", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const destroyPause = await pauseNextTerminalDestroy(testPage);
    await createTaskAndWait(apiClient, seedData, "Busy Close UI");
    const session = await openTask(testPage, "Busy Close UI");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    await clickNewTerminalInPlusMenu(testPage, session);
    const siblingTab = testPage.getByTestId("terminal-tab-seq-1").locator("..").locator("..");
    const targetTab = testPage.getByTestId("terminal-tab-seq-2").locator("..").locator("..");
    await expect(siblingTab).toBeVisible({ timeout: 10_000 });
    await expect(targetTab).toBeVisible({ timeout: 10_000 });

    const siblingTestId = await siblingTab.getAttribute("data-testid");
    expect(siblingTestId).toMatch(/^terminal-tab-shell-/);
    const stableSiblingTab = testPage.getByTestId(siblingTestId!);
    const targetTestId = await targetTab.getAttribute("data-testid");
    expect(targetTestId).toMatch(/^terminal-tab-shell-/);
    const targetId = targetTestId!.replace("terminal-tab-", "");
    const targetHost = testPage
      .locator(`[data-portal-panel="${targetId}"]`)
      .getByTestId("terminal-xterm-host");
    await expect(targetHost).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(() => readTerminalHostBuffer(targetHost), {
        timeout: 30_000,
        message: "second terminal shell should connect",
      })
      .not.toBe("");

    destroyPause.arm();
    const closeTarget = testPage.getByTestId(`terminal-tab-close-${targetId}`);
    await closeTarget.click();

    const confirmation = testPage.getByTestId("terminal-close-confirm-popover");
    await expect(confirmation).toBeVisible({ timeout: 5_000 });
    await expect(confirmation).toHaveRole("dialog");
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await expect(targetTab).toBeVisible();
    const confirmationBox = await confirmation.boundingBox();
    expect(confirmationBox).not.toBeNull();
    expect(confirmationBox!.width).toBeLessThanOrEqual(320);
    expect(confirmationBox!.height).toBeLessThanOrEqual(220);
    await confirmation.getByRole("button", { name: "Close terminal", exact: true }).click();
    await destroyPause.waitForRequest();

    // The transport is deliberately paused: disappearance must be optimistic,
    // with no tab, spinner, or popover waiting for backend teardown.
    await expect(confirmation).toBeHidden();
    await expect(targetTab).toHaveCount(0);
    // Once only one ordinary terminal remains its sequence badge disappears,
    // so use the stable shell-id test id captured before the close.
    await expect(stableSiblingTab).toBeVisible({ timeout: 5_000 });
    await stableSiblingTab.click();

    const siblingHost = testPage.getByTestId("terminal-panel").getByTestId("terminal-xterm-host");
    await expect(siblingHost).toBeVisible({ timeout: 10_000 });
    await siblingHost.click();
    await testPage.keyboard.type("printf '%s\\n' $((71820 + 3))");
    await testPage.keyboard.press("Enter");
    await expect
      .poll(() => readTerminalHostBuffer(siblingHost), {
        timeout: 10_000,
        message: "sibling terminal should remain interactive after confirmed close",
      })
      .toContain("71823");
    destroyPause.release();
  });

  /**
   * Right-click → Terminate must hard-destroy the terminal AND remove
   * its dockview tab in the same gesture. Before the fix the WS RPC
   * fired but the panel stayed open with a dead PTY and the row hung
   * around in the reopen menu until the next refresh.
   */
  test("right-click → Terminate closes the tab and removes the row", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await createTaskAndWait(apiClient, seedData, "Tab Terminate UI");
    const session = await openTask(testPage, "Tab Terminate UI");
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();

    // Add a second terminal so we have a non-default one to terminate
    // (the default's id is `terminal-default` and its right-click menu
    // routes through the same handler, but the seq=2 case is what
    // surfaced the bug).
    await clickNewTerminalInPlusMenu(testPage, session);
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toBeVisible({ timeout: 10_000 });

    // Right-click the seq=2 tab and pick Terminate.
    const seq2TabTrigger = testPage.getByTestId("terminal-tab-seq-2").locator("..").locator("..");
    await seq2TabTrigger.click({ button: "right" });
    await testPage.getByRole("menuitem", { name: /^Terminate/ }).click();

    const confirmation = testPage.getByTestId("terminal-close-confirm-popover");
    await expect(confirmation).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toBeVisible();
    await confirmation.getByRole("button", { name: "Close terminal", exact: true }).click();

    // The seq=2 badge must disappear — proves the panel was closed.
    await expect(testPage.getByTestId("terminal-tab-seq-2")).toHaveCount(0, { timeout: 5_000 });

    // The seq=2 row must NOT appear in the reopen menu — proves the
    // row was hard-destroyed, not parked.
    await session.addPanelButton().click();
    const seq2Row = testPage
      .locator('[data-testid^="reopen-terminal-"]')
      .filter({ has: testPage.getByTestId("reopen-terminal-seq-2") });
    await expect(seq2Row).toHaveCount(0, { timeout: 5_000 });
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
