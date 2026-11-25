import { expect, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  createInheritedTask,
  createOrdinaryTerminal,
  createTaskWithSession,
  expectSharedEnvironment,
  listOrdinaryTerminals,
  mobileTerminalRowIDs,
} from "./task-ownership-helpers";
import {
  focusTerminalForTyping,
  readTerminalBuffer,
  switchToTerminalPanel,
  waitForShellReady,
} from "./mobile-terminal-helpers";

async function openMobileTerminal(page: Page, taskID: string): Promise<SessionPage> {
  await page.goto(`/t/${taskID}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await switchToTerminalPanel(page);
  await waitForShellReady(page);
  return session;
}

/** Switches tasks through the phone task picker without reloading the SPA. */
async function switchMobileTask(page: Page, taskID: string): Promise<SessionPage> {
  await page.getByTestId("mobile-task-picker-trigger").tap();
  const picker = page.getByRole("dialog", { name: "Tasks", exact: true });
  await picker.locator(`[data-task-row-id="${taskID}"]`).tap();
  await expect(page).toHaveURL(new RegExp(`/t/${taskID}`));
  const session = new SessionPage(page);
  await session.waitForLoad();
  await switchToTerminalPanel(page);
  await waitForShellReady(page);
  return session;
}

async function runMobileCommand(page: Page, command: string, expected: string): Promise<void> {
  await focusTerminalForTyping(page);
  await page.keyboard.type(command, { delay: 5 });
  await page.keyboard.press("Enter");
  await expect
    .poll(async () => (await readTerminalBuffer(page)).includes(expected), {
      timeout: 15_000,
      message: `Expected the mobile terminal to contain "${expected}"`,
    })
    .toBe(true);
}

test.describe("Mobile terminal ownership — shared task environment", () => {
  test("keeps each task's own shell and picker rows across navigation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const parent = await createTaskWithSession(apiClient, seedData, "Mobile ownership parent");
    const child = await createInheritedTask(apiClient, seedData, parent, "Mobile ownership child");
    const environmentID = await expectSharedEnvironment(apiClient, parent, child);
    const foreign = await createOrdinaryTerminal(apiClient, child.id, environmentID);

    await openMobileTerminal(testPage, parent.id);
    const parentRows = await listOrdinaryTerminals(apiClient, parent.id, environmentID);
    const parentIDs = parentRows.map((row) => row.id ?? "");
    const parentPicker = await mobileTerminalRowIDs(testPage);
    expect(parentPicker.length).toBeGreaterThan(0);
    expect(parentPicker).not.toContain(foreign.id);
    for (const terminalID of parentPicker) {
      expect(parentIDs).toContain(terminalID);
    }
    await runMobileCommand(testPage, "export KANDEV_OWNER=task-a; echo exported-a", "exported-a");
    await runMobileCommand(testPage, "echo owner-a=$KANDEV_OWNER", "owner-a=task-a");

    await switchMobileTask(testPage, child.id);
    const childPicker = await mobileTerminalRowIDs(testPage);
    expect(childPicker).toContain(foreign.id);
    for (const terminalID of childPicker) {
      expect(parentIDs.includes(terminalID), "parent terminal leaked into child picker").toBe(
        false,
      );
    }
    await runMobileCommand(testPage, "echo owner-b=$KANDEV_OWNER", "owner-b=");
    await expect
      .poll(async () => (await readTerminalBuffer(testPage)).includes("owner-b=task-a"))
      .toBe(false);
    await runMobileCommand(testPage, "export KANDEV_OWNER=task-b; echo exported-b", "exported-b");
    await runMobileCommand(testPage, "echo marker-b", "marker-b");

    await switchMobileTask(testPage, parent.id);
    await runMobileCommand(testPage, "echo owner-a2=$KANDEV_OWNER", "owner-a2=task-a");

    await testPage.reload();
    const reloaded = new SessionPage(testPage);
    await reloaded.waitForLoad();
    await switchToTerminalPanel(testPage);
    await waitForShellReady(testPage);
    await runMobileCommand(testPage, "echo reload-a=$KANDEV_OWNER", "reload-a=task-a");

    // The child's terminal survives every switch untouched.
    const childAfter = await listOrdinaryTerminals(apiClient, child.id, environmentID);
    const preserved = childAfter.find((row) => row.id === foreign.id);
    expect(preserved, "the child terminal disappeared").toBeDefined();
    expect(preserved?.state).toBe("open");
    expect(preserved?.custom_name ?? null).toBeNull();
  });
});
