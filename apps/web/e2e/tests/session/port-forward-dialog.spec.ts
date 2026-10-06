import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionAgentctlReady } from "../../helpers/session-store";
import { SessionPage } from "../../pages/session-page";
import { routePortForwarding } from "./port-forwarding-helpers";

/**
 * Seed a task + session with a mock_remote executor and navigate to the session page.
 * Sets the workspace default executor to mock_remote so the session picks it up.
 */
async function seedRemoteSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ session: SessionPage; sessionId: string }> {
  // Create a mock_remote executor and set it as workspace default
  const executor = await apiClient.createExecutor("E2E Mock Remote", "mock_remote");
  await apiClient.updateWorkspace(seedData.workspaceId, {
    default_executor_id: executor.id,
  });

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

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  // waitForChatIdle (not a raw idleInput wait) rides out the WS-subscribe race:
  // the auto-started mock agent can complete before the client's WS subscription
  // registers, leaving isAgentBusy=true and the idle input never rendering. The
  // helper reloads once to re-derive state from SSR. The plain wait flaked here.
  await session.waitForChatIdle({ timeout: 30_000 });
  await waitForSessionAgentctlReady(testPage, task.session_id);

  // Reset workspace default executor so other tests aren't affected
  await apiClient.updateWorkspace(seedData.workspaceId, {
    default_executor_id: "",
  });

  return { session, sessionId: task.session_id };
}

/**
 * Seed a task + session with the default (local) executor.
 */
async function seedLocalSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ session: SessionPage; sessionId: string }> {
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

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  // See seedRemoteSession: waitForChatIdle handles the WS-subscribe race that
  // makes a raw idleInput wait flake when the auto-started agent finishes early.
  await session.waitForChatIdle({ timeout: 30_000 });
  await waitForSessionAgentctlReady(testPage, task.session_id);

  return { session, sessionId: task.session_id };
}

test.describe("Port Forward Dialog", () => {
  // @covers AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.2, .4
  test("merges late tunnel hydration with a newly started forward", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const ports = await routePortForwarding(testPage);
    ports.holdNext("port.tunnel.list");
    const { session, sessionId } = await seedLocalSession(
      testPage,
      apiClient,
      seedData,
      "Late tunnel snapshot",
    );
    ports.setSession(sessionId);
    await session.enablePortForwarding();
    await expect.poll(ports.held).toBe(true);
    await session.portForwardButton.click();
    await session.portForwardInput.fill("9500");
    await session.portForwardAddButton.click();
    await session.portForwardTunnelToggle(9500).click();
    await session.portForwardTunnelStart(9500).click();
    await expect(session.portForwardRow(9500)).toHaveAttribute("data-forwarded", "true");
    ports.release();
    await expect(session.portForwardDialog.getByTestId("port-forward-active-heading")).toHaveText(
      "Forwarded ports2",
    );
    await expect(session.portForwardRow(9000).getByRole("link").first()).toHaveAttribute(
      "href",
      /:49152\/$/,
    );
    await expect(session.portForwardRow(9500).getByRole("link").first()).toHaveAttribute(
      "href",
      /:49153\/$/,
    );
    await session.portForwardDialog.press("Escape");
  });

  // @covers AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.1-.5, .7
  test("prioritizes forwarded ports and preserves focus through start and stop", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const ports = await routePortForwarding(testPage);
    const { session, sessionId } = await seedLocalSession(
      testPage,
      apiClient,
      seedData,
      "Active-first ports",
    );
    ports.setSession(sessionId);
    await session.enablePortForwarding();
    await session.portForwardButton.press("Enter");
    await expect(session.portForwardRefresh).toBeFocused();
    await testPage.keyboard.press("Tab");
    await expect(session.portForwardTunnelToggle(9000)).toBeFocused();
    const rows = session.portForwardDialog.locator('[data-testid^="port-forward-row-"]');
    const ids = () =>
      rows.evaluateAll((elements) =>
        elements.map((element) => element.getAttribute("data-testid")),
      );
    await expect.poll(ids).toEqual(["port-forward-row-9000", "port-forward-row-3000"]);
    await expect(session.portForwardDialog.getByTestId("port-forward-active-heading")).toHaveText(
      "Forwarded ports1",
    );
    await expect(
      session.portForwardRow(9000).getByText("Forwarding", { exact: true }),
    ).toBeVisible();
    await expect(session.portForwardRow(9000).getByRole("link").first()).toHaveAttribute(
      "href",
      /:49152\/$/,
    );
    await session.portForwardInput.fill("9500");
    await session.portForwardAddButton.click();
    await session.portForwardTunnelToggle(9500).click();
    await session.portForwardTunnelStart(9500).click();
    await expect
      .poll(ids)
      .toEqual(["port-forward-row-9000", "port-forward-row-9500", "port-forward-row-3000"]);
    await expect(session.portForwardTunnelToggle(9500)).toBeFocused();
    ports.failNext("port.tunnel.stop");
    await session.portForwardTunnelToggle(9500).press("Enter");
    await expect(
      testPage.getByText("Failed to stop tunnel: Port operation failed", { exact: true }),
    ).toBeVisible();
    await expect(session.portForwardRow(9500)).toHaveAttribute("data-forwarded", "true");
    await expect(session.portForwardTunnelToggle(9500)).toHaveAttribute("aria-disabled", "false");
    await session.portForwardTunnelToggle(9500).press("Enter");
    await expect
      .poll(ids)
      .toEqual(["port-forward-row-9000", "port-forward-row-3000", "port-forward-row-9500"]);
    await expect(session.portForwardTunnelToggle(9500)).toBeFocused();
    await testPage.screenshot({ path: test.info().outputPath("active-first-desktop.png") });
    await testPage.keyboard.press("Escape");
    // Wait for the tooltip layer to unmount before dismissing its parent dialog.
    await expect(testPage.locator('[data-slot="tooltip-content"]')).toHaveCount(0);
    if (
      await session.portForwardDialog.evaluateAll((elements) =>
        elements.some((element) => element.getAttribute("data-state") === "open"),
      )
    ) {
      await testPage.keyboard.press("Escape");
    }
    await expect(session.portForwardDialog).toBeHidden();
    await expect(session.portForwardButton).toBeFocused();
  });

  test("local executor can enable the port forwarding control", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedLocalSession(testPage, apiClient, seedData, "Local Port Test");
    await expect(session.portForwardButton).not.toBeVisible();
    await session.enablePortForwarding();
    await expect(session.portForwardButton).toBeVisible();
  });

  test("button is visible for mock remote executor after launcher enable", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedRemoteSession(testPage, apiClient, seedData, "Remote Port Test");
    await expect(session.portForwardButton).toBeHidden();
    await session.enablePortForwarding();
    await expect(session.portForwardButton).toBeVisible();
  });

  test("disabling the preference hides the top-bar control", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedRemoteSession(
      testPage,
      apiClient,
      seedData,
      "Disable Port Forwarding Test",
    );
    await session.enablePortForwarding();
    await expect(session.portForwardButton).toBeVisible();

    await session.togglePortForwardingPreference();

    await expect(session.portForwardButton).not.toBeVisible();
  });

  test("disabling visibility leaves an active tunnel available", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedRemoteSession(
      testPage,
      apiClient,
      seedData,
      "Active Tunnel Visibility Test",
    );
    await session.enablePortForwarding();
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();

    await session.portForwardInput.fill("3000");
    await session.portForwardAddButton.click();
    const row = session.portForwardRow(3000);
    await expect(row).toBeVisible();
    await session.portForwardTunnelToggle(3000).click();
    await session.portForwardTunnelStart(3000).click();
    await expect(row.locator("a[target='_blank']")).toHaveCount(2);

    await session.portForwardDialog.getByRole("button", { name: "Close" }).click();
    await session.togglePortForwardingPreference();
    await expect(session.portForwardButton).not.toBeVisible();

    await session.togglePortForwardingPreference();
    await expect(session.portForwardButton).toBeVisible();
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();
    await expect(session.portForwardRow(3000).locator("a[target='_blank']")).toHaveCount(2);
    await session.portForwardDialog.getByRole("button", { name: "Close" }).click();
  });

  test("preference survives a task page reload", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedLocalSession(
      testPage,
      apiClient,
      seedData,
      "Persist Port Forwarding Test",
    );
    await session.enablePortForwarding();

    await testPage.reload();
    await session.waitForLoad();

    await expect(session.portForwardButton).toBeVisible();
  });

  test("dialog opens on button click", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(testPage, apiClient, seedData, "Dialog Open Test");
    await session.enablePortForwarding();
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();
  });

  test("auto-refresh loads port list on open", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(testPage, apiClient, seedData, "Refresh Test");
    await session.enablePortForwarding();
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();
    // Dialog auto-refreshes on open; wait for the placeholder to disappear,
    // meaning either ports were detected or the "no ports" message appeared.
    // The E2E host may have real listening ports, so we accept either outcome.
    const noPortsMessage = session.portForwardDialog.getByText("No listening ports detected");
    const detectedBadge = session.portForwardDialog.getByText("Detected").first();
    await expect(noPortsMessage.or(detectedBadge)).toBeVisible({ timeout: 10_000 });
  });

  test("add manual port shows row with Manual badge", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(testPage, apiClient, seedData, "Manual Port Test");
    await session.enablePortForwarding();
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();

    await session.portForwardInput.fill("3000");
    await session.portForwardAddButton.click();

    const row = session.portForwardRow(3000);
    await expect(row).toBeVisible();
    await expect(row.getByText("Manual")).toBeVisible();
  });

  test("opens the proxy URL in the existing Browser panel", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedRemoteSession(
      testPage,
      apiClient,
      seedData,
      "Open Proxy Browser Panel Test",
    );
    await session.enablePortForwarding();
    await session.addBrowserPanel();
    await expect(session.browserPanel).toBeVisible();
    const browserPanelCount = await testPage.getByTestId("browser-panel").count();
    expect(browserPanelCount).toBe(1);

    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();
    await session.portForwardInput.fill("3000");
    await session.portForwardAddButton.click();

    const row = session.portForwardRow(3000);
    const proxyUrl = await row.locator("a[target='_blank']").getAttribute("href");
    expect(proxyUrl).toBeTruthy();
    await session.portForwardOpenBrowser(3000).click();

    await expect(session.portForwardDialog).toBeHidden();
    await expect(session.browserPanel).toBeVisible();
    await expect(testPage.getByTestId("browser-panel")).toHaveCount(browserPanelCount);
    await expect(session.browserAddressInput).toHaveValue(proxyUrl!);
    await expect(session.browserPanel.locator("iframe")).toHaveAttribute("src", proxyUrl!);
  });

  test("opens the proxy URL in a new Browser panel", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(
      testPage,
      apiClient,
      seedData,
      "Open Proxy In New Browser Panel Test",
    );
    await session.enablePortForwarding();

    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();
    await session.portForwardInput.fill("3000");
    await session.portForwardAddButton.click();

    const row = session.portForwardRow(3000);
    const proxyUrl = await row.locator("a[target='_blank']").getAttribute("href");
    expect(proxyUrl).toBeTruthy();
    await session.portForwardOpenBrowser(3000).click();

    await expect(session.portForwardDialog).toBeHidden();
    await expect(session.browserPanel).toBeVisible();
    await expect(session.browserAddressInput).toHaveValue(proxyUrl!);
    await expect(session.browserPanel.locator("iframe")).toHaveAttribute("src", proxyUrl!);
  });

  test("rejects invalid port number", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(testPage, apiClient, seedData, "Invalid Port Test");
    await session.enablePortForwarding();
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();

    await session.portForwardInput.fill("99999");
    await session.portForwardAddButton.click();

    // Invalid port should not create a row
    await expect(session.portForwardRow(99999)).not.toBeVisible();
  });

  test("rejects duplicate port", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(
      testPage,
      apiClient,
      seedData,
      "Duplicate Port Test",
    );
    await session.enablePortForwarding();
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();

    await session.portForwardInput.fill("8080");
    await session.portForwardAddButton.click();
    await expect(session.portForwardRow(8080)).toBeVisible();

    // Try adding the same port again — should still have only one row
    await session.portForwardInput.fill("8080");
    await session.portForwardAddButton.click();
    const rows = session.portForwardDialog.locator('[data-testid="port-forward-row-8080"]');
    await expect(rows).toHaveCount(1);
  });

  test("manual port row has correct proxy URL", async ({ testPage, apiClient, seedData }) => {
    const { session, sessionId } = await seedRemoteSession(
      testPage,
      apiClient,
      seedData,
      "Proxy URL Test",
    );
    await session.enablePortForwarding();
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();

    await session.portForwardInput.fill("5000");
    await session.portForwardAddButton.click();

    const row = session.portForwardRow(5000);
    await expect(row).toBeVisible();

    const link = row.locator("a[target='_blank']");
    const href = await link.getAttribute("href");
    expect(href).toContain(`/port-proxy/${sessionId}/5000/`);
  });

  test("Enter key submits port", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedRemoteSession(testPage, apiClient, seedData, "Enter Key Test");
    await session.enablePortForwarding();
    await session.portForwardButton.click();
    await expect(session.portForwardDialog).toBeVisible();

    await session.portForwardInput.fill("4000");
    await session.portForwardInput.press("Enter");

    await expect(session.portForwardRow(4000)).toBeVisible();
  });
});
