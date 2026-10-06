import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  createMcpRecoveryFixture,
  destroyMcpRecoveryTerminals,
  E2E_MCP_SERVER_ID,
  installAgentMcpRecoveryRoutes,
} from "../../helpers/agent-mcp-preparation";
import { selectAgentIfNeeded } from "./quick-chat-helpers";

test.describe("Setup recovery UX (mobile)", () => {
  test("shows authentication warning with accessible touch targets on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const fixture = await createMcpRecoveryFixture(apiClient, seedData, "Agent MCP Warning Mobile");
    await installAgentMcpRecoveryRoutes(testPage, {
      sessionId: fixture.sessionId,
      taskEnvironmentId: fixture.taskEnvironmentId,
      terminalId: fixture.authenticationTerminalId,
    });

    try {
      await testPage.goto(`/t/${fixture.taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      const preparation = testPage.getByTestId("prepare-progress-panel");
      await expect(preparation).toHaveAttribute("data-status", "completed_with_warnings");
      await expect(preparation).toHaveAttribute("data-expanded", "true");

      await expect(preparation).toContainText(`Verify connection: ${E2E_MCP_SERVER_ID}`);
      await expect(preparation.getByTestId("prepare-step-warning-icon")).toBeVisible();

      const actions = preparation.getByTestId("agent-mcp-recovery-actions");
      await expect(actions).toBeVisible();
      const authMessage = actions.getByText("Authentication is required.");
      await expect(authMessage).toBeVisible();
      await expect(authMessage).toHaveClass(/text-amber-700/);

      const authenticate = actions.getByTestId("agent-mcp-authenticate");
      await expect(authenticate).toBeVisible();
      const buttonBox = await authenticate.boundingBox();
      expect(buttonBox).not.toBeNull();
      expect(buttonBox!.height).toBeGreaterThanOrEqual(44);

      // Check no horizontal document overflow
      const scrollWidth = await testPage.evaluate(() => document.documentElement.scrollWidth);
      const clientWidth = await testPage.evaluate(() => document.documentElement.clientWidth);
      expect(scrollWidth).toBeLessThanOrEqual(clientWidth);
    } finally {
      await destroyMcpRecoveryTerminals(apiClient, fixture);
    }
  });

  test("displays inline error on mobile with accessible touch targets", async ({ testPage }) => {
    await testPage.route("**/quick-chat*", async (route) => {
      if (route.request().method() !== "POST") {
        await route.continue();
        return;
      }
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "Invalid mobile repository configuration" }),
      });
    });

    await testPage.goto("/");
    await expect(testPage.getByTestId("app-nav-trigger")).toBeVisible();
    await testPage.getByTestId("app-nav-trigger").tap();
    await testPage.getByTestId("mobile-quick-chat-button").tap();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible({ timeout: 10_000 });
    await selectAgentIfNeeded(dialog, testPage);

    const openingPrompt = "Keep this request available for retry.";
    const prompt = dialog.getByTestId("task-description-input");
    await prompt.fill(openingPrompt);
    const sendButton = dialog.getByTestId("quick-chat-send");
    await expect(sendButton).toBeEnabled({ timeout: 10_000 });
    await sendButton.tap();

    const setupError = dialog.getByTestId("quick-chat-setup-error");
    await expect(setupError).toBeVisible({ timeout: 10_000 });
    await expect(setupError).toContainText("Invalid mobile repository configuration");
    await expect(prompt).toHaveValue(openingPrompt);
    await expect(sendButton).toBeEnabled();

    const sendBox = await sendButton.boundingBox();
    expect(sendBox).not.toBeNull();
    expect(sendBox!.height).toBeGreaterThanOrEqual(44);

    const scrollWidth = await testPage.evaluate(() => document.documentElement.scrollWidth);
    const clientWidth = await testPage.evaluate(() => document.documentElement.clientWidth);
    expect(scrollWidth).toBeLessThanOrEqual(clientWidth);
  });
});
