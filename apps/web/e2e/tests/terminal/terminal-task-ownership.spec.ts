import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import {
  contaminateDefaultTerminalPanel,
  createInheritedTask,
  createOrdinaryTerminal,
  createTaskWithSession,
  defaultPanelTerminalID,
  expectSharedEnvironment,
  expectTerminalMissing,
  listOrdinaryTerminals,
  openTaskTerminal,
  runTerminalCommand,
  type TerminalRow,
} from "./task-ownership-helpers";
import { pauseNextUserShellCreate } from "./user-shell-create-pause";

test.describe("Terminal ownership — shared task environment", () => {
  test("each task keeps its own shell across switching and reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const parent = await createTaskWithSession(apiClient, seedData, "Ownership parent task");
    const child = await createInheritedTask(apiClient, seedData, parent, "Ownership child task");
    const environmentID = await expectSharedEnvironment(apiClient, parent, child);

    const parentSession = await openTaskTerminal(testPage, parent.id);
    await runTerminalCommand(
      parentSession,
      "export KANDEV_OWNER=task-a; echo exported-a",
      "exported-a",
    );
    await runTerminalCommand(parentSession, "echo owner-a=$KANDEV_OWNER", "owner-a=task-a");
    await runTerminalCommand(parentSession, "echo marker-a", "marker-a");

    const childSession = await openTaskTerminal(testPage, child.id);
    // A fresh shell for task B must not inherit task A's exported value. If
    // the panel attached to task A's PTY this prints task-a and fails.
    await runTerminalCommand(childSession, "echo owner-b=$KANDEV_OWNER", "owner-b=");
    await expectTerminalMissing(childSession, "owner-b=task-a");
    await runTerminalCommand(
      childSession,
      "export KANDEV_OWNER=task-b; echo exported-b",
      "exported-b",
    );
    await runTerminalCommand(childSession, "echo marker-b", "marker-b");

    const childRows = await listOrdinaryTerminals(apiClient, child.id, environmentID);
    expect(childRows.length).toBeGreaterThan(0);

    const parentAgain = await openTaskTerminal(testPage, parent.id);
    await runTerminalCommand(parentAgain, "echo owner-a2=$KANDEV_OWNER", "owner-a2=task-a");

    const afterReload = await openTaskTerminal(testPage, parent.id);
    await afterReload.clickTab("Terminal");
    await runTerminalCommand(afterReload, "echo reload-a=$KANDEV_OWNER", "reload-a=task-a");

    const parentRows = await listOrdinaryTerminals(apiClient, parent.id, environmentID);
    const parentIDs = new Set(parentRows.map((row) => row.id));
    for (const row of childRows) {
      expect(parentIDs.has(row.id), "child terminal leaked into the parent list").toBe(false);
    }
    const childIDs = new Set(childRows.map((row) => row.id));
    for (const row of parentRows) {
      expect(childIDs.has(row.id), "parent terminal leaked into the child list").toBe(false);
    }

    // No mutation may have crossed task ownership.
    const childAfter = await listOrdinaryTerminals(apiClient, child.id, environmentID);
    const preserved = childAfter.find((row) => row.id === childRows[0].id);
    expect(preserved, "the child terminal disappeared").toBeDefined();
    expect(preserved?.state).toBe("open");
    expect(preserved?.custom_name ?? null).toBeNull();
  });

  test("does not publish a late terminal create into the task the user switched to", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const parent = await createTaskWithSession(apiClient, seedData, "Late create parent");
    const child = await createInheritedTask(apiClient, seedData, parent, "Late create child");
    const environmentID = await expectSharedEnvironment(apiClient, parent, child);

    const createPause = await pauseNextUserShellCreate(testPage);
    const session = await openTaskTerminal(testPage, parent.id);
    await runTerminalCommand(session, "echo marker-late", "marker-late");
    const beforeRows = await listOrdinaryTerminals(apiClient, parent.id, environmentID);
    const beforeIDs = new Set(beforeRows.map((row) => row.id));

    createPause.arm();
    await session.addPanelButton().click();
    await testPage.getByTestId("new-terminal-button").click();
    await createPause.waitForRequest();

    // Switch tasks without a reload, so the pending create promise and its
    // captured panel publisher survive.
    await session.sidebar.getByTestId("sidebar-task-item").filter({ hasText: child.title }).click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${child.id}`));
    await session.waitForLoad();
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();
    const childPanelsBefore = await testPage.evaluate(() => {
      type Panel = {
        id: string;
        params?: Record<string, unknown>;
        api?: { isActive?: boolean };
      };
      type Api = { panels: Panel[] };
      return ((window as unknown as { __dockviewApi__?: Api }).__dockviewApi__?.panels ?? []).map(
        (panel) => ({
          id: panel.id,
          params: panel.params ?? {},
          active: panel.api?.isActive ?? false,
        }),
      );
    });

    createPause.release();
    let created: TerminalRow | undefined;
    await expect
      .poll(
        async () => {
          const rows = await listOrdinaryTerminals(apiClient, parent.id, environmentID);
          created = rows.find((row) => !beforeIDs.has(row.id));
          return created !== undefined;
        },
        { timeout: 15_000, message: "the parent create should still land in its own cache" },
      )
      .toBe(true);

    // Give the late completion a layout-save window; the guard is synchronous,
    // so a published panel would already be present.
    await dwell(
      testPage,
      750,
      "negative-assertion",
      "late create publication is synchronous; this window only lets a refetch-driven save settle",
    );

    const createdID = created!.id!;
    const childPanelsAfter = await testPage.evaluate(() => {
      type Panel = {
        id: string;
        params?: Record<string, unknown>;
        api?: { isActive?: boolean };
      };
      type Api = { panels: Panel[] };
      return ((window as unknown as { __dockviewApi__?: Api }).__dockviewApi__?.panels ?? []).map(
        (panel) => ({
          id: panel.id,
          params: panel.params ?? {},
          active: panel.api?.isActive ?? false,
        }),
      );
    });
    expect(childPanelsAfter).toEqual(childPanelsBefore);
    expect(childPanelsAfter.some((panel) => panel.id === createdID)).toBe(false);
    expect(childPanelsAfter.some((panel) => panel.params.terminalId === createdID)).toBe(false);

    // Returning to the origin task exposes the created row from its cache.
    await session.sidebar
      .getByTestId("sidebar-task-item")
      .filter({ hasText: parent.title })
      .click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${parent.id}`));
    await session.waitForLoad();
    await session.addPanelButton().click();
    await expect(testPage.getByTestId(`reopen-terminal-${createdID}`)).toBeVisible();
    await testPage.keyboard.press("Escape");
  });

  test("repairs a restored default panel pointing at another task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const parent = await createTaskWithSession(apiClient, seedData, "Contaminated parent task");
    const child = await createInheritedTask(apiClient, seedData, parent, "Contaminated child task");
    const environmentID = await expectSharedEnvironment(apiClient, parent, child);
    const foreign = await createOrdinaryTerminal(apiClient, child.id, environmentID);

    const session = await openTaskTerminal(testPage, parent.id);
    await runTerminalCommand(session, "echo marker-parent", "marker-parent");
    const parentRows = await listOrdinaryTerminals(apiClient, parent.id, environmentID);
    expect(parentRows.some((row) => row.id === foreign.id)).toBe(false);

    await contaminateDefaultTerminalPanel(testPage, environmentID, foreign.id!);
    await testPage.reload();
    await session.waitForLoad();

    await expect
      .poll(
        async () => {
          const terminalID = await defaultPanelTerminalID(testPage);
          return terminalID !== null && parentRows.some((row) => row.id === terminalID);
        },
        {
          timeout: 30_000,
          message: "default panel should recover from the foreign terminal id",
        },
      )
      .toBe(true);

    const repairedID = await defaultPanelTerminalID(testPage);
    expect(repairedID).not.toBe(foreign.id);
    await session.clickTab("Terminal");
    await session.expectTerminalConnected();
    await runTerminalCommand(session, "echo recovered-parent", "recovered-parent");

    // Recovery must not remove, rename, or park the foreign terminal.
    const childRows = await listOrdinaryTerminals(apiClient, child.id, environmentID);
    const preserved = childRows.find((row) => row.id === foreign.id);
    expect(preserved, "recovery removed another task's terminal").toBeDefined();
    expect(preserved?.state).toBe("open");
    expect(preserved?.custom_name ?? null).toBeNull();
  });
});
