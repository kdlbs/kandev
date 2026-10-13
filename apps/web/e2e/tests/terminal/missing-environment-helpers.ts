import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { dwell } from "../../helpers/causal-waits";
import { switchToTerminalPanel, waitForShellReady } from "./mobile-terminal-helpers";

export function registerMissingEnvironmentTerminalTest(mobile: boolean) {
  test("a retained terminal stops reconnecting after its task is deleted", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    let retainBrowserState = false;
    await testPage.route("**/api/v1/**", async (route) => {
      if (retainBrowserState) {
        await route.abort("internetdisconnected");
        return;
      }
      const response = await route.fetch();
      if (retainBrowserState) await route.abort("internetdisconnected");
      else await route.fulfill({ response });
    });
    await testPage.routeWebSocket(/\/ws$/, (socket) => {
      const server = socket.connectToServer();
      server.onMessage((message) => {
        // Retain cached session state, including status polls, while the
        // independent terminal transport reconnects to the removed target.
        if (!retainBrowserState) socket.send(message);
      });
    });
    const attempts: string[] = [];
    const closeOutcomes: { code: number; reason: string }[] = [];
    let disconnectTerminal = () => {};
    await testPage.routeWebSocket(/\/terminal\/environment\//, (socket) => {
      attempts.push(socket.url());
      const server = socket.connectToServer();
      server.onClose((code, reason) => {
        closeOutcomes.push({ code, reason });
        socket.close({ code, reason });
      });
      disconnectTerminal = () => {
        socket.close({ code: 1012, reason: "restart" });
        server.close();
      };
    });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Retained deleted terminal",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    if (mobile) {
      await switchToTerminalPanel(testPage);
      await waitForShellReady(testPage);
    } else {
      await session.expectTerminalConnected(30_000);
      await session.expectTerminalShellReady();
    }
    const panel = testPage.locator('[data-testid="terminal-panel"]:visible');
    await expect(panel).toHaveCount(1);
    await expect(panel.getByTestId("passthrough-session-ended")).toBeHidden();
    const initialAttempts = attempts.length;
    expect(initialAttempts).toBeGreaterThan(0);
    retainBrowserState = true;
    try {
      await apiClient.deleteTask(task.id, { discardWorktreeChanges: true });
      // Model the retained pane reconnecting after a backend/network restart.
      disconnectTerminal();
      await expect
        .poll(() => closeOutcomes)
        .toContainEqual({ code: 1000, reason: "environment_not_found" });
      await expect(panel.getByTestId("passthrough-session-ended")).toBeVisible();
      await expect(panel.getByTestId("passthrough-loading")).toBeHidden();
      const settledAttempts = attempts.length;
      await dwell(
        testPage,
        6_000,
        "negative-assertion",
        "a deleted environment must stay disconnected beyond the five-second retry cap",
      );
      expect(attempts).toHaveLength(settledAttempts);
      await prCapture.screenshot(mobile ? "mobile-ended-terminal" : "desktop-ended-terminal", {
        caption: "A deleted terminal environment shows ended feedback and stops reconnecting.",
      });
    } finally {
      retainBrowserState = false;
    }
  });
}
