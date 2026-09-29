import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  createMcpRecoveryFixture,
  destroyMcpRecoveryTerminals,
  E2E_MCP_SERVER_ID,
  installAgentMcpRecoveryRoutes,
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
    await expect(preparation).toHaveAttribute("data-status", "completed_with_error");
    await preparation.getByRole("button", { name: "Show preparation details" }).tap();
    await expect(preparation).toContainText("Discover servers: plugin-atlassian-jira");
    await expect(preparation).toContainText("Verify connection: plugin-atlassian-jira");

    await testPage.reload();
    await session.waitForLoad();
    const hydratedPreparation = testPage.getByTestId("prepare-progress-panel");
    await expect(hydratedPreparation).toHaveAttribute("data-status", "completed_with_error");
    await hydratedPreparation.getByRole("button", { name: "Show preparation details" }).tap();
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
      .poll(() =>
        terminalSocketUrls.some((socketUrl) => {
          const url = new URL(socketUrl);
          return (
            url.pathname.endsWith(`/terminal/environment/${fixture.taskEnvironmentId}`) &&
            url.searchParams.get("terminalId") === fixture.authenticationTerminalId
          );
        }),
      )
      .toBe(true);
    await expect(
      testPage.getByTestId(`mobile-terminal-slot-${fixture.existingTerminalId}`),
    ).toHaveAttribute("data-state", "inactive");
    expect(fixture.authenticationTerminalId).not.toBe(fixture.existingTerminalId);
    expect(requests.authenticate).toEqual([{ server_id: E2E_MCP_SERVER_ID }]);
    await expect(testPage.getByTestId("terminal-panel")).toBeVisible();
  } finally {
    await destroyMcpRecoveryTerminals(apiClient, fixture);
  }
});
