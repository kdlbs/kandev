import { type Locator, type Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { dwell, waitForHttp } from "../../helpers/causal-waits";
import {
  closeQuickTerminals,
  openDesktopQuickTerminal,
  readQuickTerminalDescriptor,
  waitForTerminalText,
} from "./quick-terminal-recovery-helpers";

const QUICK_CHAT_TITLE = "Quick Chat";

async function selectedTerminal(dialog: Locator) {
  const panel = dialog.getByTestId("quick-terminal-tab-panel");
  await expect(panel).toBeVisible();
  const terminal = panel.getByTestId("quick-terminal-terminal");
  await expect(terminal).toBeVisible();
  return { panel, terminal };
}

async function sendShellCommand(page: Page, terminal: Locator, command: string) {
  await terminal.click();
  await page.keyboard.type(command);
  await page.keyboard.press("Enter");
}

async function createExitedTerminal(
  page: Page,
  apiClient: Parameters<typeof readQuickTerminalDescriptor>[0],
  workspaceId: string,
  dialog: Locator,
) {
  const { panel, terminal } = await selectedTerminal(dialog);
  const tabId = await panel.getAttribute("data-terminal-tab-id");
  if (!tabId) throw new Error("Quick Terminal tab is missing its stable ID");
  await expect
    .poll(async () => (await readQuickTerminalDescriptor(apiClient, workspaceId, tabId)).status)
    .toBe("running");
  const before = await readQuickTerminalDescriptor(apiClient, workspaceId, tabId);
  expect(before.status).toBe("running");
  await sendShellCommand(page, terminal, "exit");
  await expect
    .poll(async () => {
      const descriptor = await readQuickTerminalDescriptor(apiClient, workspaceId, tabId);
      return descriptor.status === "exited" && descriptor.sessionId === null;
    })
    .toBe(true);
  return { tabId, before };
}

async function recoverAndVerify({
  page,
  apiClient,
  workspaceId,
  dialog,
  tabId,
  oldDescriptor,
}: {
  page: Page;
  apiClient: Parameters<typeof readQuickTerminalDescriptor>[0];
  workspaceId: string;
  dialog: Locator;
  tabId: string;
  oldDescriptor: Awaited<ReturnType<typeof readQuickTerminalDescriptor>>;
}) {
  const button = dialog.getByRole("button", { name: "Start new shell", exact: true });
  await expect(button).toBeVisible();
  await button.click();
  await expect(dialog.getByTestId("quick-terminal-tab-panel")).toBeVisible();
  const { terminal } = await selectedTerminal(dialog);
  await sendShellCommand(page, terminal, "printf 'RECOVERED%s\\n' '_SHELL'");
  await waitForTerminalText(terminal, "RECOVERED_SHELL");
  const recovered = await readQuickTerminalDescriptor(apiClient, workspaceId, tabId);
  expect(recovered.tabId).toBe(oldDescriptor.tabId);
  expect(recovered.sequence).toBe(oldDescriptor.sequence);
  expect(recovered.status).toBe("running");
  expect(recovered.sessionId).toBeTruthy();
  expect(recovered.sessionId).not.toBe(oldDescriptor.sessionId);
  await expect(button).toHaveCount(0);
  return recovered;
}

test.describe("quick terminal shell recovery", () => {
  test("starts a fresh shell after natural exit in the same tab", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    try {
      const dialog = await openDesktopQuickTerminal(testPage);
      const { tabId, before } = await createExitedTerminal(
        testPage,
        apiClient,
        seedData.workspaceId,
        dialog,
      );
      await recoverAndVerify({
        page: testPage,
        apiClient,
        workspaceId: seedData.workspaceId,
        dialog,
        tabId,
        oldDescriptor: before,
      });
    } finally {
      await closeQuickTerminals(testPage);
    }
  });

  test("recovers a persisted tab after backend restart and page reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    try {
      const initialDialog = await openDesktopQuickTerminal(testPage);
      const tabs = initialDialog.locator('[data-testid="quick-terminal-tab"]');
      await expect(tabs).toHaveCount(1);
      const tabId = await initialDialog
        .getByTestId("quick-terminal-tab-panel")
        .getAttribute("data-terminal-tab-id");
      if (!tabId) throw new Error("Quick Terminal tab is missing its stable ID");
      const original = await readQuickTerminalDescriptor(apiClient, seedData.workspaceId, tabId);

      await backend.restart();
      await expect
        .poll(async () => {
          const descriptor = await readQuickTerminalDescriptor(
            apiClient,
            seedData.workspaceId,
            tabId,
          );
          return descriptor.sessionId === null && descriptor.status === "exited";
        })
        .toBe(true);
      await testPage.reload();
      const dialog = await openDesktopQuickTerminal(testPage);
      await expect(
        dialog.getByRole("button", { name: "Start new shell", exact: true }),
      ).toBeVisible();
      const lost = await readQuickTerminalDescriptor(apiClient, seedData.workspaceId, tabId);
      expect(lost.sequence).toBe(original.sequence);
      await recoverAndVerify({
        page: testPage,
        apiClient,
        workspaceId: seedData.workspaceId,
        dialog,
        tabId,
        oldDescriptor: original,
      });
    } finally {
      await closeQuickTerminals(testPage);
    }
  });

  test("recovers an open browser terminal after the required restart reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    try {
      const dialog = await openDesktopQuickTerminal(testPage);
      const tabId = await dialog
        .getByTestId("quick-terminal-tab-panel")
        .getAttribute("data-terminal-tab-id");
      if (!tabId) throw new Error("Quick Terminal tab is missing its stable ID");
      const original = await readQuickTerminalDescriptor(apiClient, seedData.workspaceId, tabId);

      await testPage.waitForLoadState("networkidle");
      const systemInfoRecovered = waitForHttp(testPage, "GET", /^\/api\/v1\/system\/info$/, {
        predicate: (response) => response.ok(),
      });
      await backend.restart();
      await systemInfoRecovered;
      const reload = testPage.getByTestId("backend-reload-required-alert");
      await expect(reload).toBeVisible();
      await testPage.keyboard.press("Escape");
      await expect(dialog).toHaveCount(0);
      await reload.getByRole("button", { name: "Reload page", exact: true }).click();
      await expect(testPage.getByTestId("app-shell")).toBeVisible();
      await expect(reload).toHaveCount(0);
      await testPage.getByTestId("sidebar-quick-terminal-shortcut").click();
      await expect(
        dialog.getByRole("button", { name: "Start new shell", exact: true }),
      ).toBeVisible();
      await recoverAndVerify({
        page: testPage,
        apiClient,
        workspaceId: seedData.workspaceId,
        dialog,
        tabId,
        oldDescriptor: original,
      });
    } finally {
      await closeQuickTerminals(testPage);
    }
  });

  test("adopts a live shell another client recovered for the descriptor", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    try {
      const dialog = await openDesktopQuickTerminal(testPage);
      const { tabId, before } = await createExitedTerminal(
        testPage,
        apiClient,
        seedData.workspaceId,
        dialog,
      );
      const response = await apiClient.rawRequest("POST", "/api/v1/host-shell/start", {
        cols: 80,
        rows: 24,
        client_id: tabId,
      });
      expect(response.ok).toBe(true);
      const otherSession = (await response.json()) as { session_id: string };
      const recoverButton = dialog.getByRole("button", { name: "Start new shell", exact: true });
      await recoverButton.click();
      const { terminal } = await selectedTerminal(dialog);
      await sendShellCommand(testPage, terminal, "printf 'ADOPTED%s\\n' '_SHELL'");
      await waitForTerminalText(terminal, "ADOPTED_SHELL");
      const descriptor = await readQuickTerminalDescriptor(apiClient, seedData.workspaceId, tabId);
      expect(descriptor.sessionId).toBe(otherSession.session_id);
      expect(descriptor.sequence).toBe(before.sequence);
    } finally {
      await closeQuickTerminals(testPage);
    }
  });

  test("keeps a live sibling attached while recovering only the exited tab", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    try {
      const dialog = await openDesktopQuickTerminal(testPage);
      const siblingTab = dialog.locator('[data-testid="quick-terminal-tab"]').nth(0);
      const siblingId = await dialog
        .getByTestId("quick-terminal-tab-panel")
        .getAttribute("data-terminal-tab-id");
      const siblingSequence = await siblingTab.getAttribute("data-terminal-sequence");
      if (!siblingId) throw new Error("Sibling tab is missing its stable ID");
      if (!siblingSequence) throw new Error("Sibling tab is missing its sequence");
      const siblingTerminal = (await selectedTerminal(dialog)).terminal;
      await sendShellCommand(testPage, siblingTerminal, "export KANDEV_QT_SIBLING=LIVE_SIBLING");
      const siblingDescriptorBefore = await readQuickTerminalDescriptor(
        apiClient,
        seedData.workspaceId,
        siblingId,
      );
      const siblingSessionId = siblingDescriptorBefore.sessionId;
      if (!siblingSessionId) throw new Error("Sibling shell did not receive a session ID");

      await dialog.getByTestId("quick-chat-add-menu-trigger").click();
      await testPage.getByTestId("quick-chat-new-terminal").click();
      const tabs = dialog.locator('[data-testid="quick-terminal-tab"]');
      await expect(tabs).toHaveCount(2);
      const exited = await createExitedTerminal(testPage, apiClient, seedData.workspaceId, dialog);
      const recovered = await recoverAndVerify({
        page: testPage,
        apiClient,
        workspaceId: seedData.workspaceId,
        dialog,
        tabId: exited.tabId,
        oldDescriptor: exited.before,
      });

      await dialog
        .locator(`[data-testid="quick-terminal-tab"][data-terminal-sequence="${siblingSequence}"]`)
        .getByRole("button", { name: `Terminal ${siblingSequence}`, exact: true })
        .click();
      const siblingPanel = (await selectedTerminal(dialog)).terminal;
      await sendShellCommand(
        testPage,
        siblingPanel,
        "printf 'SIBLING%s\\n' \"$KANDEV_QT_SIBLING\"",
      );
      await waitForTerminalText(siblingPanel, "SIBLINGLIVE_SIBLING");
      const siblingDescriptor = await readQuickTerminalDescriptor(
        apiClient,
        seedData.workspaceId,
        siblingId,
      );
      expect(siblingDescriptor.sessionId).toBe(siblingSessionId);
      expect(siblingDescriptor.sessionId).not.toBe(recovered.sessionId);
      expect(siblingDescriptor.status).toBe("running");
    } finally {
      await closeQuickTerminals(testPage);
    }
  });

  test("shows breakpoint and coarse-pointer recovery targets at the required size", async ({
    testPage,
    apiClient,
    seedData,
    tabletTestPage,
  }) => {
    test.setTimeout(120_000);
    try {
      const dialog = await openDesktopQuickTerminal(testPage);
      await createExitedTerminal(testPage, apiClient, seedData.workspaceId, dialog);
      await testPage.setViewportSize({ width: 767, height: 900 });
      const action = dialog.getByRole("button", { name: "Start new shell", exact: true });
      await expect(action).toBeVisible();
      expect((await action.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      await testPage.setViewportSize({ width: 768, height: 900 });
      expect((await action.boundingBox())!.height).toBe(28);
      await testPage.setViewportSize({ width: 1280, height: 900 });
      await closeQuickTerminals(testPage);

      await tabletTestPage.goto("/");
      await tabletTestPage.getByTestId("tablet-quick-terminal-button").click();
      const tabletDialog = tabletTestPage.getByRole("dialog", { name: QUICK_CHAT_TITLE });
      await expect(tabletDialog).toBeVisible();
      const tabletTabId = await tabletDialog
        .getByTestId("quick-terminal-tab-panel")
        .getAttribute("data-terminal-tab-id");
      if (!tabletTabId) throw new Error("Tablet tab is missing its stable ID");
      const tabletTerminal = (await selectedTerminal(tabletDialog)).terminal;
      await sendShellCommand(tabletTestPage, tabletTerminal, "exit");
      await expect
        .poll(async () => {
          const descriptor = await readQuickTerminalDescriptor(
            apiClient,
            seedData.workspaceId,
            tabletTabId,
          );
          return descriptor.status === "exited";
        })
        .toBe(true);
      const touchButton = tabletDialog.getByRole("button", {
        name: "Start new shell",
        exact: true,
      });
      expect((await touchButton.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    } finally {
      await closeQuickTerminals(testPage);
      await closeQuickTerminals(tabletTestPage);
    }
  });

  test("shows a failed start and allows an explicit retry without a retry loop", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    let releaseFailure!: () => void;
    let starts = 0;
    const failureHeld = new Promise<void>((resolve) => {
      releaseFailure = resolve;
    });
    const route = "**/api/v1/host-shell/start";
    try {
      const dialog = await openDesktopQuickTerminal(testPage);
      const { tabId, before } = await createExitedTerminal(
        testPage,
        apiClient,
        seedData.workspaceId,
        dialog,
      );
      await testPage.route(route, async (requestRoute) => {
        starts += 1;
        if (starts === 1) {
          await failureHeld;
          await requestRoute.fulfill({
            status: 500,
            contentType: "application/json",
            body: JSON.stringify({ error: "controlled shell start failure" }),
          });
          return;
        }
        await requestRoute.continue();
      });
      const button = dialog.getByRole("button", { name: "Start new shell", exact: true });
      await expect(button).toBeVisible();
      await button.click();
      await expect(button).toBeDisabled();
      await expect(dialog.getByTestId("quick-terminal-status")).toContainText(
        "Connecting to terminal",
      );
      await button.evaluate((element) => {
        element.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      });
      await expect.poll(() => starts).toBe(1);
      releaseFailure();
      await expect(dialog.getByRole("alert")).toContainText("controlled shell start failure");
      await expect(button).toBeEnabled();
      await dwell(
        testPage,
        1_000,
        "negative-assertion",
        "confirm failed Quick Terminal recovery does not retry automatically",
      );
      expect(starts).toBe(1);
      await button.click();
      await expect.poll(() => starts).toBe(2);
      const { terminal } = await selectedTerminal(dialog);
      await sendShellCommand(testPage, terminal, "printf 'RETRY%s\\n' '_WORKED'");
      await waitForTerminalText(terminal, "RETRY_WORKED");
      const after = await readQuickTerminalDescriptor(apiClient, seedData.workspaceId, tabId);
      expect(after.tabId).toBe(before.tabId);
      expect(after.sequence).toBe(before.sequence);
      expect(after.sessionId).not.toBe(before.sessionId);
    } finally {
      releaseFailure();
      await testPage.unroute(route).catch(() => undefined);
      await closeQuickTerminals(testPage);
    }
  });
});
