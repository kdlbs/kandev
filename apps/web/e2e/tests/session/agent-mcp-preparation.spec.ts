import { expect, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  createMcpRecoveryFixture,
  destroyMcpRecoveryTerminals,
  E2E_MCP_SERVER_ID,
  E2E_MCP_SECOND_SERVER_ID,
  E2E_MCP_LEGACY_ERROR,
  E2E_MCP_LEGACY_OUTPUT,
  installAgentMcpRecoveryRoutes,
  seedCursorMcpDiscoveryPreparation,
  seedMcpDiagnosticPreparation,
} from "../../helpers/agent-mcp-preparation";
import { routeGatewayNotifications } from "../../helpers/ws-capture";

type McpRecoveryFixture = Awaited<ReturnType<typeof createMcpRecoveryFixture>>;
type GatewayNotifications = Awaited<ReturnType<typeof routeGatewayNotifications>>;

async function exerciseLiveCursorMcpDiscoveryFeedback(
  testPage: Page,
  notifications: GatewayNotifications,
  fixture: McpRecoveryFixture,
) {
  const preparation = testPage.getByTestId("prepare-progress-panel");
  const common = {
    task_id: fixture.taskId,
    session_id: fixture.sessionId,
    execution_id: "e2e-cursor-discovery-execution",
    step_name: "",
    step_kind: "agent_mcp_discovery",
    mcp_provider: "cursor",
    step_index: 0,
    total_steps: 3,
    status: "failed",
    preparation_id: "e2e-live-cursor-discovery-attempt",
    preparation_started_at: "2101-01-01T00:00:00.000000001Z",
    timestamp: new Date().toISOString(),
  };
  notifications.send("executor.prepare.progress", { ...common, failure_code: "timeout" });

  const timeoutDetails = preparation.getByTestId("cursor-mcp-discovery-details");
  await expect(preparation).toHaveAttribute("data-status", "preparing");
  await expect(preparation).toContainText("Discover Cursor MCP servers");
  await expect(timeoutDetails).toBeVisible();

  const setupCollapseToggle = preparation.getByRole("button", {
    name: "Hide preparation details",
  });
  await setupCollapseToggle.click();
  await expect(preparation).toHaveAttribute("data-expanded", "false");
  const setupExpandToggle = preparation.getByRole("button", {
    name: "Show preparation details",
  });
  await expect(setupExpandToggle).toBeFocused();
  await setupExpandToggle.click();
  await expect(preparation).toHaveAttribute("data-expanded", "true");
  await expect(setupCollapseToggle).toBeFocused();

  const timeoutTrigger = timeoutDetails.getByRole("button", { name: "Details" });
  await testPage.keyboard.press("Tab");
  await expect(timeoutTrigger).toBeFocused();
  await testPage.keyboard.press("Enter");
  await expect(timeoutTrigger).toHaveAttribute("aria-expanded", "true");
  await expect(timeoutTrigger).toBeFocused();
  await expect(timeoutDetails).toContainText(
    "Checking enabled Cursor Marketplace plugins timed out (5-second limit).",
  );
  await expect(timeoutDetails).toContainText(
    "Kandev skipped Marketplace plugin imports for this setup attempt. If setup completes, other configured MCP servers can still connect.",
  );
  await testPage.keyboard.press("Space");
  await expect(timeoutTrigger).toHaveAttribute("aria-expanded", "false");
  await expect(timeoutTrigger).toBeFocused();

  notifications.send("executor.prepare.progress", {
    ...common,
    failure_code: "credentials_unavailable",
  });
  await timeoutTrigger.click();
  await expect(timeoutDetails).toContainText(
    "The lookup could not obtain Cursor account credentials.",
  );
  await timeoutTrigger.click();

  notifications.send("executor.prepare.progress", {
    ...common,
    failure_code: "unknown_reason",
  });
  await timeoutTrigger.click();
  await expect(timeoutDetails).toContainText(
    "Kandev could not complete Cursor MCP discovery for this attempt.",
  );
  await timeoutTrigger.click();

  await testPage.setViewportSize({ width: 420, height: 900 });
  await testPage.evaluate(() => {
    document.documentElement.style.fontSize = "14px";
  });
  const narrowBox = await timeoutTrigger.boundingBox();
  expect(narrowBox?.height).toBeGreaterThanOrEqual(44);
  await testPage.evaluate(() => {
    document.documentElement.style.removeProperty("font-size");
  });
  await expect(timeoutDetails).not.toHaveAttribute("data-failure-code");

  notifications.send("executor.prepare.progress", { ...common, failure_code: "canceled" });
  await timeoutTrigger.click();
  await expect(timeoutDetails).toContainText(
    "The check for enabled Cursor Marketplace plugins was canceled.",
  );
  await expect(timeoutDetails).not.toContainText(
    "Kandev skipped Marketplace plugin imports for this setup attempt.",
  );
  await timeoutTrigger.click();
}

async function expectHydratedCursorMcpDiscoveryFeedback(testPage: Page) {
  const preparation = testPage.getByTestId("prepare-progress-panel");
  await expect(preparation).toHaveAttribute("data-status", "completed_with_error");
  await preparation.getByRole("button", { name: "Show preparation details" }).click();
  await expect(preparation).toContainText("Discover Cursor MCP servers");
  await expect(preparation).toContainText("Verify connection: kandev-diagnostics");
  const details = preparation.getByTestId("cursor-mcp-discovery-details");
  await details.getByRole("button", { name: "Details" }).click();
  await expect(details).toContainText(
    "Checking enabled Cursor Marketplace plugins timed out (5-second limit).",
  );
  await expect(preparation).toContainText("Verify connection: kandev-diagnostics");
}

test.describe("Agent MCP preparation recovery", () => {
  test("opens the exact DB-backed authentication terminal and retries in the same session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const fixture = await createMcpRecoveryFixture(
      apiClient,
      seedData,
      "Agent MCP Preparation Recovery",
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
      await expect(preparation).toContainText("Prepare workspace");
      await expect(preparation).toContainText("Discover Cursor MCP servers: plugin-atlassian-jira");
      await expect(preparation).toContainText("Apply profile selection: plugin-atlassian-jira");
      await expect(preparation).toContainText("Check credentials: plugin-atlassian-jira");
      await expect(preparation).toContainText("Approve server: plugin-atlassian-jira");
      await expect(preparation).toContainText("Verify connection: plugin-atlassian-jira");

      await testPage.reload();
      await session.waitForLoad();
      const hydratedPreparation = testPage.getByTestId("prepare-progress-panel");
      await expect(hydratedPreparation).toHaveAttribute("data-status", "completed_with_warnings");
      await expect(hydratedPreparation).toHaveAttribute("data-expanded", "true");
      await expect(hydratedPreparation).toContainText("Verify connection: plugin-atlassian-jira");
      const actions = hydratedPreparation.getByTestId("agent-mcp-recovery-actions");
      await expect(actions).toContainText("Authentication is required.");

      await actions.getByTestId("agent-mcp-authenticate").click();
      await expect(
        actions.getByRole("status").filter({ hasText: "Sign-in terminal opened." }),
      ).toContainText("Sign-in terminal opened.");
      const authenticationTerminalTab = testPage.getByTestId(
        `terminal-tab-${fixture.authenticationTerminalId}`,
      );
      await expect(authenticationTerminalTab).toBeVisible();
      await expect(
        testPage.locator(".dv-tab.dv-active-tab", { has: authenticationTerminalTab }),
      ).toBeVisible();
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
        testPage.locator('[data-testid="terminal-panel"]:visible').first(),
      ).toBeVisible();
      expect(fixture.authenticationTerminalId).not.toBe(fixture.existingTerminalId);
      expect(requests.authenticate).toEqual([{ server_id: E2E_MCP_SERVER_ID }]);
      await expect
        .poll(async () => {
          const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
          return sessions.filter((item) => item.id === fixture.sessionId).length;
        })
        .toBe(1);

      await actions.getByTestId("agent-mcp-retry").click();
      await expect(actions.getByRole("status").filter({ hasText: "Connection ready." })).toHaveText(
        "Connection ready.",
      );
      expect(requests.retry).toEqual([{ server_id: E2E_MCP_SERVER_ID }]);
      const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
      expect(sessions.map((item) => item.id)).toContain(fixture.sessionId);
      expect(sessions).toHaveLength(1);
    } finally {
      await destroyMcpRecoveryTerminals(apiClient, fixture);
    }
  });

  test("retains command details through reload and preserves another server after busy and ready retries", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(90_000);
    const fixture = await createMcpRecoveryFixture(
      apiClient,
      seedData,
      "Agent MCP Diagnostic Recovery",
    );
    await seedMcpDiagnosticPreparation(apiClient, seedData, fixture);
    const requests = await installAgentMcpRecoveryRoutes(testPage, {
      sessionId: fixture.sessionId,
      taskEnvironmentId: fixture.taskEnvironmentId,
      terminalId: fixture.authenticationTerminalId,
      retryResponses: [
        {
          status: 409,
          body: {
            error: "The agent is using this session.",
            error_code: "mcp_recovery_session_busy",
          },
        },
        {
          status: 200,
          body: {
            provider_id: "cursor",
            server_id: E2E_MCP_SERVER_ID,
            status: "ready",
            tool_count: 3,
          },
        },
      ],
    });

    try {
      await testPage.goto(`/t/${fixture.taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      let preparation = testPage.getByTestId("prepare-progress-panel");
      await expect(preparation).toHaveAttribute("data-status", "failed");
      await testPage.getByRole("button", { name: "Show preparation details" }).click();

      let primaryRow = testPage.locator(
        `[data-testid="agent-mcp-recovery-actions"][data-mcp-server-id="${E2E_MCP_SERVER_ID}"]`,
      );
      let otherRow = testPage.locator(
        `[data-testid="agent-mcp-recovery-actions"][data-mcp-server-id="${E2E_MCP_SECOND_SERVER_ID}"]`,
      );
      await expect(primaryRow).toContainText("Native command failed during server approval.");
      await expect(primaryRow).toContainText("exec: WaitDelay expired before I/O complete");
      await expect(primaryRow).toContainText("Exit status: 0");
      await expect(primaryRow).toContainText("Cleanup error: process cleanup failed");
      await expect(otherRow).toContainText("Native MCP connection verification command failed");
      for (const row of [primaryRow, otherRow]) {
        await expect(row).not.toContainText(E2E_MCP_LEGACY_ERROR);
        await expect(row).not.toContainText(E2E_MCP_LEGACY_OUTPUT);
      }
      await testPage.screenshot({ path: testInfo.outputPath("agent-mcp-diagnostic-desktop.png") });

      await testPage.reload();
      await session.waitForLoad();
      preparation = testPage.getByTestId("prepare-progress-panel");
      await expect(preparation).toHaveAttribute("data-status", "failed");
      await testPage.getByRole("button", { name: "Show preparation details" }).click();
      await expect(primaryRow).toContainText("exec: WaitDelay expired before I/O complete");
      await expect(primaryRow).toContainText("Cleanup error: process cleanup failed");
      await expect(otherRow).toContainText("Native MCP connection verification command failed");
      for (const row of [primaryRow, otherRow]) {
        await expect(row).not.toContainText(E2E_MCP_LEGACY_ERROR);
        await expect(row).not.toContainText(E2E_MCP_LEGACY_OUTPUT);
      }

      await testPage.setViewportSize({ width: 420, height: 900 });
      const narrowPreparation = testPage.locator('[data-testid="prepare-progress-panel"]:visible');
      await expect(narrowPreparation).toHaveCount(1);
      if ((await narrowPreparation.getAttribute("data-expanded")) !== "true") {
        await narrowPreparation.getByRole("button", { name: "Show preparation details" }).click();
      }
      const narrowOtherRow = narrowPreparation.locator(
        `[data-testid="agent-mcp-recovery-actions"][data-mcp-server-id="${E2E_MCP_SECOND_SERVER_ID}"]`,
      );
      const narrowRetry = narrowOtherRow.getByTestId("agent-mcp-retry");
      await expect(narrowRetry).toBeVisible();
      const narrowRetryBox = await narrowRetry.boundingBox();
      expect(narrowRetryBox?.height).toBeGreaterThanOrEqual(44);
      const narrowDocument = await testPage.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth,
        clientWidth: document.documentElement.clientWidth,
      }));
      expect(narrowDocument.scrollWidth).toBeLessThanOrEqual(narrowDocument.clientWidth);

      await testPage.setViewportSize({ width: 1280, height: 900 });
      preparation = testPage.locator('[data-testid="prepare-progress-panel"]:visible');
      await expect(preparation).toHaveCount(1);
      if ((await preparation.getAttribute("data-expanded")) !== "true") {
        await preparation.getByRole("button", { name: "Show preparation details" }).click();
      }
      primaryRow = preparation.locator(
        `[data-testid="agent-mcp-recovery-actions"][data-mcp-server-id="${E2E_MCP_SERVER_ID}"]`,
      );
      otherRow = preparation.locator(
        `[data-testid="agent-mcp-recovery-actions"][data-mcp-server-id="${E2E_MCP_SECOND_SERVER_ID}"]`,
      );

      const desktopRetry = primaryRow.getByTestId("agent-mcp-retry");
      const desktopRetryBox = await desktopRetry.boundingBox();
      expect(desktopRetryBox?.height).toBeGreaterThanOrEqual(27);
      expect(desktopRetryBox?.height).toBeLessThanOrEqual(29);
      await desktopRetry.click();
      await expect(primaryRow).toContainText(
        "The agent is using this session. Wait for its current turn to finish, then retry.",
      );
      await expect(primaryRow).toContainText("exec: WaitDelay expired before I/O complete");

      await desktopRetry.click();
      await expect(primaryRow.getByRole("status").last()).toHaveText("Connection ready.");
      await expect(primaryRow.getByTestId("agent-mcp-diagnostic")).toHaveCount(0);
      await expect(otherRow).toContainText("Native MCP connection verification command failed");
      expect(requests.retry).toEqual([
        { server_id: E2E_MCP_SERVER_ID },
        { server_id: E2E_MCP_SERVER_ID },
      ]);
    } finally {
      await destroyMcpRecoveryTerminals(apiClient, fixture);
    }
  });

  test("explains a Cursor MCP discovery timeout during setup and after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const notifications = await routeGatewayNotifications(testPage);
    const fixture = await createMcpRecoveryFixture(
      apiClient,
      seedData,
      "Cursor MCP Discovery Feedback",
    );

    try {
      await testPage.goto(`/t/${fixture.taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect.poll(() => notifications.isConnected()).toBe(true);

      await exerciseLiveCursorMcpDiscoveryFeedback(testPage, notifications, fixture);

      await seedCursorMcpDiscoveryPreparation(apiClient, seedData, fixture);
      await testPage.reload();
      await session.waitForLoad();
      await expectHydratedCursorMcpDiscoveryFeedback(testPage);
    } finally {
      await destroyMcpRecoveryTerminals(apiClient, fixture);
    }
  });
});
