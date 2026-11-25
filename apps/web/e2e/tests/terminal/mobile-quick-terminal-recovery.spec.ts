import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { closeQuickTerminalTab } from "./terminal-test-helpers";
import {
  closeQuickTerminals,
  openMobileQuickTerminal,
  readQuickTerminalDescriptor,
  waitForTerminalText,
} from "./quick-terminal-recovery-helpers";

test.describe("mobile Quick Terminal shell recovery", () => {
  // @covers AC-UI-QUICK-TERMINAL-001.15, AC-UI-QUICK-TERMINAL-001.16, AC-UI-QUICK-TERMINAL-001.19
  test("recovers an exited shell from the phone menu and keeps the action reachable above the keyboard", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    try {
      const dialog = await openMobileQuickTerminal(testPage);
      const tabs = dialog.locator('[data-testid="quick-terminal-tab"]');
      await expect(tabs).toHaveCount(1);
      const tabId = await dialog
        .getByTestId("quick-terminal-tab-panel")
        .getAttribute("data-terminal-tab-id");
      if (!tabId) throw new Error("Quick Terminal tab is missing its stable ID");
      const panel = dialog.getByTestId("quick-terminal-tab-panel");
      const terminal = panel.getByTestId("quick-terminal-terminal");
      await expect(terminal).toBeVisible();
      const before = await readQuickTerminalDescriptor(apiClient, seedData.workspaceId, tabId);

      await terminal.tap();
      await testPage.keyboard.type("exit");
      await testPage.keyboard.press("Enter");
      await expect
        .poll(async () => {
          const descriptor = await readQuickTerminalDescriptor(
            apiClient,
            seedData.workspaceId,
            tabId,
          );
          return descriptor.status === "exited" && descriptor.sessionId === null;
        })
        .toBe(true);

      const status = dialog.getByTestId("quick-terminal-status");
      const recovery = dialog.getByRole("button", { name: "Start new shell", exact: true });
      await expect(status).toContainText("Terminal exited");
      await expect(recovery).toBeVisible();
      const recoveryBox = await recovery.boundingBox();
      const dialogBox = await dialog.boundingBox();
      const statusBox = await status.boundingBox();
      expect(recoveryBox).not.toBeNull();
      expect(dialogBox).not.toBeNull();
      expect(statusBox).not.toBeNull();
      expect(recoveryBox!.height).toBeGreaterThanOrEqual(44);
      expect(recoveryBox!.width).toBeGreaterThanOrEqual(44);
      expect(recoveryBox!.x).toBeGreaterThanOrEqual(dialogBox!.x);
      expect(recoveryBox!.x + recoveryBox!.width).toBeLessThanOrEqual(
        dialogBox!.x + dialogBox!.width,
      );
      expect(recoveryBox!.y).toBeGreaterThan(statusBox!.y);
      await prCapture.screenshot("quick-terminal-recovery-phone-idle", {
        caption: "Quick Terminal recovery remains inside the selected phone tab",
      });

      await testPage.evaluate(() => {
        const viewport = window.visualViewport;
        if (!viewport) return;
        Object.defineProperty(viewport, "height", {
          configurable: true,
          value: window.innerHeight - 300,
        });
        viewport.dispatchEvent(new Event("resize"));
      });
      await expect
        .poll(async () => {
          const box = await recovery.boundingBox();
          return box ? box.y + box.height : Number.POSITIVE_INFINITY;
        })
        .toBeLessThanOrEqual(testPage.viewportSize()!.height - 300 + 1);

      await recovery.tap();
      await expect(dialog.getByTestId("quick-terminal-terminal")).toBeVisible();
      await testPage.evaluate(() => {
        const viewport = window.visualViewport;
        if (!viewport) return;
        Object.defineProperty(viewport, "height", {
          configurable: true,
          value: window.innerHeight,
        });
        viewport.dispatchEvent(new Event("resize"));
      });
      const recoveredTerminal = panel.getByTestId("quick-terminal-terminal");
      await recoveredTerminal.tap();
      await testPage.keyboard.type("printf 'RECOVERED%s\\n' '_SHELL'");
      await testPage.keyboard.press("Enter");
      await waitForTerminalText(recoveredTerminal, "RECOVERED_SHELL", true);
      const after = await readQuickTerminalDescriptor(apiClient, seedData.workspaceId, tabId);
      expect(after.tabId).toBe(before.tabId);
      expect(after.sequence).toBe(before.sequence);
      expect(after.status).toBe("running");
      expect(after.sessionId).toBeTruthy();
      expect(after.sessionId).not.toBe(before.sessionId);
      await expect(recovery).toHaveCount(0);
      await assertNoDocumentHorizontalOverflow(testPage, "mobile Quick Terminal recovery");
      await prCapture.screenshot("quick-terminal-recovery-phone-running", {
        caption: "Recovered Quick Terminal executes a command in the same phone tab",
      });

      await closeQuickTerminalTab(testPage, tabs.nth(0));
      await expect(tabs).toHaveCount(0);
    } finally {
      await closeQuickTerminals(testPage, true);
    }
  });
});
