import { expect, test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  routeMainWebSocketWithExpiredPluginSnapshot,
  routeMainWebSocketWithPromptDrop,
} from "../../helpers/ws-drop";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";

const PANEL_KEY = "prompt-history-plugin";

async function createSeededConversation(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{ taskId: string; sessionId: string }> {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    description: "conversation recovery fixture",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "IDLE",
    agentProfileId: seedData.agentProfileId,
    repositoryId: seedData.repositoryId,
  });
  return { taskId: task.id, sessionId };
}

async function uninstallFixturePlugin(apiClient: ApiClient): Promise<void> {
  await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
}

test.describe("Conversation recovery", () => {
  test.afterEach(async ({ apiClient }) => uninstallFixturePlugin(apiClient));

  test("repairs core messages and turns after a multi-event gap on the live socket", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const wsDrop = await routeMainWebSocketWithPromptDrop(testPage);
    let gatewayCloseCount = 0;
    testPage.on("websocket", (socket) => {
      if (!socket.url().endsWith("/ws")) return;
      socket.on("close", () => {
        gatewayCloseCount += 1;
      });
    });

    const { taskId, sessionId } = await createSeededConversation(
      apiClient,
      seedData,
      "Core conversation recovery",
    );
    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const skippedPrompt = "core recovery skipped prompt";
    const replayedPrompt = "core recovery replayed prompt";
    wsDrop.dropPrompt(skippedPrompt);
    const skipped = await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: skippedPrompt,
      authorType: "user",
      newTurn: true,
      turnStartedAt: "2026-09-14T12:00:00Z",
      turnCompletedAt: "2026-09-14T12:00:02Z",
    });
    const replayed = await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: replayedPrompt,
      authorType: "user",
      newTurn: true,
      turnStartedAt: "2026-09-14T12:01:00Z",
      turnCompletedAt: "2026-09-14T12:01:03Z",
    });

    await expect
      .poll(wsDrop.droppedCount, {
        timeout: 10_000,
        message: "expected the first durable message event to be dropped",
      })
      .toBeGreaterThan(0);
    await expect
      .poll(wsDrop.recoveryResponseCount, {
        timeout: 15_000,
        message: "expected core recovery snapshots after the ordered gap",
      })
      .toBeGreaterThan(0);

    const chat = session.activeChat();
    await expect(chat.locator(`#msg-${skipped.messageId}`)).toBeVisible({ timeout: 15_000 });
    await expect(chat.locator(`#msg-${replayed.messageId}`)).toBeVisible({ timeout: 15_000 });
    await expect(
      chat.locator(`#msg-${skipped.messageId}`).getByTestId("message-turn-duration"),
    ).toBeVisible();
    await expect(
      chat.locator(`#msg-${replayed.messageId}`).getByTestId("message-turn-duration"),
    ).toBeVisible();

    const laterLive = await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: "core recovery later live update",
      authorType: "user",
      newTurn: true,
      turnStartedAt: "2026-09-14T12:02:00Z",
      turnCompletedAt: "2026-09-14T12:02:01Z",
    });
    await expect(chat.locator(`#msg-${laterLive.messageId}`)).toBeVisible({ timeout: 15_000 });
    expect(gatewayCloseCount, "core recovery must keep the gateway socket connected").toBe(0);
  });

  test("rebinds an expired plugin continuation and keeps paging without remount", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await installFixturePlugin(testPage);
    const expiry = await routeMainWebSocketWithExpiredPluginSnapshot(testPage);
    const renewalRequests: string[] = [];
    testPage.on("request", (request) => {
      if (request.url().includes("/conversation/continuation/renew")) {
        renewalRequests.push(request.url());
      }
    });

    const { taskId, sessionId } = await createSeededConversation(
      apiClient,
      seedData,
      "Plugin continuation recovery",
    );
    for (const [index, content] of [
      "plugin recovery oldest prompt",
      "plugin recovery middle prompt",
      "plugin recovery newest prompt",
    ].entries()) {
      await apiClient.seedSessionMessage(sessionId, {
        type: "message",
        content,
        authorType: "user",
        newTurn: true,
        turnStartedAt: `2026-09-14T12:0${index}:00Z`,
        turnCompletedAt: `2026-09-14T12:0${index}:01Z`,
      });
    }

    await testPage.goto(`/t/${taskId}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    expiry.expireNextPluginSnapshot();
    await session.addPanelButton().click();
    const panelMenuItem = session.addPanelPluginItem(PLUGIN_ID, PANEL_KEY);
    await expect(panelMenuItem).toBeVisible();
    await panelMenuItem.click();

    const panel = testPage.getByTestId("fixture-prompt-history-panel");
    await expect(panel).toBeVisible({ timeout: 15_000 });
    await expect(panel.getByTestId("fixture-prompt-history-row")).toHaveCount(2);
    await expect.poll(expiry.modifiedCount).toBe(1);

    await panel.getByTestId("fixture-prompt-history-load-more").click();
    await expect
      .poll(expiry.pluginSubscribeCount, {
        timeout: 15_000,
        message: "expected a fresh plugin subscription after expiry",
      })
      .toBeGreaterThan(1);
    await expect(panel.getByTestId("fixture-prompt-history-row")).toHaveCount(3);
    expect(renewalRequests).toHaveLength(0);

    const laterLive = await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: "plugin recovery later live prompt",
      authorType: "user",
      newTurn: true,
      turnStartedAt: "2026-09-14T12:03:00Z",
      turnCompletedAt: "2026-09-14T12:03:01Z",
    });
    await expect(panel.locator(`[data-message-id="${laterLive.messageId}"]`)).toBeVisible({
      timeout: 15_000,
    });
  });
});
