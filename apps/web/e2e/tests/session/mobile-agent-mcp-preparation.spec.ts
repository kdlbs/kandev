import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  createMcpRecoveryFixture,
  destroyMcpRecoveryTerminals,
  E2E_MCP_SERVER_ID,
  E2E_MCP_LONG_DIAGNOSTIC,
  installAgentMcpRecoveryRoutes,
  seedMcpDiagnosticPreparation,
} from "../../helpers/agent-mcp-preparation";

test("phone recovery opens the exact MCP sign-in terminal in the terminal panel", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(90_000);
  const fixture = await createMcpRecoveryFixture(
    apiClient,
    seedData,
    "Mobile Agent MCP Preparation Recovery",
  );
  const requests = await installAgentMcpRecoveryRoutes(testPage, {
    sessionId: fixture.sessionId,
    taskEnvironmentId: fixture.taskEnvironmentId,
    terminalId: fixture.authenticationTerminalId,
  });
  const terminalSocketUrls: string[] = [];
  testPage.on("websocket", (socket) => terminalSocketUrls.push(socket.url()));

  try {
    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const preparation = testPage.getByTestId("prepare-progress-panel");
    await expect(preparation).toHaveAttribute("data-status", "completed_with_warnings");
    await expect(preparation).toHaveAttribute("data-expanded", "true");
    await expect(preparation).toContainText("Discover servers: plugin-atlassian-jira");
    await expect(preparation).toContainText("Verify connection: plugin-atlassian-jira");

    await testPage.reload();
    await session.waitForLoad();
    const hydratedPreparation = testPage.getByTestId("prepare-progress-panel");
    await expect(hydratedPreparation).toHaveAttribute("data-status", "completed_with_warnings");
    await expect(hydratedPreparation).toHaveAttribute("data-expanded", "true");
    await expect(hydratedPreparation).toContainText("Verify connection: plugin-atlassian-jira");
    const authenticate = hydratedPreparation.getByTestId("agent-mcp-authenticate");
    const buttonBox = await authenticate.boundingBox();
    expect(buttonBox).not.toBeNull();
    expect(buttonBox!.height).toBeGreaterThanOrEqual(44);

    await authenticate.tap();
    const authenticationTerminal = testPage.getByTestId(
      `mobile-terminal-slot-${fixture.authenticationTerminalId}`,
    );
    await expect(authenticationTerminal).toHaveAttribute("data-state", "active", {
      timeout: 15_000,
    });
    await expect(authenticationTerminal.getByTestId("terminal-xterm-host")).toBeVisible();
    await expect
      .poll(
        () =>
          terminalSocketUrls.some((socketUrl) => {
            const url = new URL(socketUrl);
            return (
              url.pathname.endsWith(`/terminal/environment/${fixture.taskEnvironmentId}`) &&
              url.searchParams.get("terminalId") === fixture.authenticationTerminalId
            );
          }),
        { timeout: 15_000 },
      )
      .toBe(true);
    await expect(
      testPage.getByTestId(`mobile-terminal-slot-${fixture.existingTerminalId}`),
    ).toHaveAttribute("data-state", "inactive");
    expect(fixture.authenticationTerminalId).not.toBe(fixture.existingTerminalId);
    expect(requests.authenticate).toEqual([{ server_id: E2E_MCP_SERVER_ID }]);
    await expect(testPage.locator('[data-testid="terminal-panel"]:visible').first()).toBeVisible();
  } finally {
    await destroyMcpRecoveryTerminals(apiClient, fixture);
  }
});

test("phone wraps retained MCP command details and keeps recovery actions reachable", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  test.setTimeout(90_000);
  const fixture = await createMcpRecoveryFixture(
    apiClient,
    seedData,
    "Mobile Agent MCP Diagnostic Recovery",
  );
  await seedMcpDiagnosticPreparation(apiClient, seedData, fixture, { longMessage: true });
  const requests = await installAgentMcpRecoveryRoutes(testPage, {
    sessionId: fixture.sessionId,
    taskEnvironmentId: fixture.taskEnvironmentId,
    terminalId: fixture.authenticationTerminalId,
  });

  try {
    await testPage.goto(`/t/${fixture.taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const preparation = testPage.getByTestId("prepare-progress-panel");
    await expect(preparation).toHaveAttribute("data-status", "failed");
    await testPage.getByRole("button", { name: "Show preparation details" }).tap();

    const primaryRow = testPage.locator(
      `[data-testid="agent-mcp-recovery-actions"][data-mcp-server-id="${E2E_MCP_SERVER_ID}"]`,
    );
    await expect(primaryRow).toContainText(E2E_MCP_LONG_DIAGNOSTIC);
    await expect(primaryRow).toContainText("Cleanup error: process cleanup failed");
    const details = primaryRow.getByTestId("agent-mcp-diagnostic-message");
    await expect(details).toBeVisible();
    expect(await details.evaluate((element) => getComputedStyle(element).whiteSpace)).toBe(
      "pre-wrap",
    );
    const detailBox = await details.boundingBox();
    const rowBox = await primaryRow.boundingBox();
    expect(detailBox).not.toBeNull();
    expect(rowBox).not.toBeNull();
    expect(detailBox!.x).toBeGreaterThanOrEqual(rowBox!.x);
    expect(detailBox!.x + detailBox!.width).toBeLessThanOrEqual(rowBox!.x + rowBox!.width + 1);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
    await testPage.screenshot({ path: testInfo.outputPath("agent-mcp-diagnostic-phone.png") });

    const retry = primaryRow.getByTestId("agent-mcp-retry");
    const retryBox = await retry.boundingBox();
    expect(retryBox?.height).toBeGreaterThanOrEqual(44);
    await retry.tap();
    await expect(primaryRow.getByRole("status").last()).toHaveText("Connection ready.");
    await expect(primaryRow.getByTestId("agent-mcp-diagnostic")).toHaveCount(0);
    expect(requests.retry).toEqual([{ server_id: E2E_MCP_SERVER_ID }]);
  } finally {
    await destroyMcpRecoveryTerminals(apiClient, fixture);
  }
});
