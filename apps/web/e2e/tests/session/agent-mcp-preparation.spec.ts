import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  createMcpRecoveryFixture,
  destroyMcpRecoveryTerminals,
  E2E_MCP_SERVER_ID,
  installAgentMcpRecoveryRoutes,
} from "../../helpers/agent-mcp-preparation";

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
      await expect(preparation).toContainText("Discover servers: plugin-atlassian-jira");
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
      await expect(actions.getByRole("status")).toContainText("Sign-in terminal opened.");
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
      await expect(actions.getByRole("status")).toHaveText("Connection ready.");
      expect(requests.retry).toEqual([{ server_id: E2E_MCP_SERVER_ID }]);
      const { sessions } = await apiClient.listTaskSessions(fixture.taskId);
      expect(sessions.map((item) => item.id)).toContain(fixture.sessionId);
      expect(sessions).toHaveLength(1);
    } finally {
      await destroyMcpRecoveryTerminals(apiClient, fixture);
    }
  });
});
