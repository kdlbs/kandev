import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";

const PANEL_KEY = "prompt-history-plugin";

async function uninstall(apiClient: ApiClient) {
  await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
}

test.describe("Packaged prompt-history fixture", () => {
  test.afterEach(async ({ apiClient }) => uninstall(apiClient));

  test("uses only Host conversation capabilities for paging, live transitions, and navigation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await installFixturePlugin(testPage);
    await apiClient.createPrompt("daily", "Saved daily prompt preview");
    const task = await apiClient.createTask(seedData.workspaceId, "Prompt history plugin parity", {
      description: "fixture initial prompt",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: seedData.agentProfileId,
      repositoryId: seedData.repositoryId,
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: "fixture oldest prompt",
      authorType: "user",
      newTurn: true,
      turnStartedAt: "2026-09-07T11:59:00Z",
      turnCompletedAt: "2026-09-07T11:59:01Z",
    });
    const older = await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: "fixture older prompt",
      authorType: "user",
      newTurn: true,
      turnStartedAt: "2026-09-07T12:00:00Z",
      turnCompletedAt: "2026-09-07T12:00:01Z",
    });
    const live = await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: "fixture live prompt @daily",
      authorType: "user",
      metadata: { sender_task_id: "fixture-agent-task" },
      newTurn: true,
      turnStartedAt: "2026-09-07T12:01:00Z",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.addPanelButton().click();
    const menuItem = session.addPanelPluginItem(PLUGIN_ID, PANEL_KEY);
    await expect(menuItem).toHaveText(/Prompt history fixture/);
    await menuItem.click();

    const panel = testPage.getByTestId("fixture-prompt-history-panel");
    await expect(panel).toBeVisible({ timeout: 10_000 });
    await expect(panel.getByTestId("fixture-prompt-history-row").first()).toContainText("Prompt");
    const loadOlder = panel.getByTestId("fixture-prompt-history-load-more");
    if (await loadOlder.isVisible()) await loadOlder.click();
    await expect(panel.locator(`[data-message-id="${live.messageId}"]`)).toContainText(
      "Sent by agent",
    );
    if (!live.turnId) throw new Error("live fixture message did not create a turn");
    await apiClient.completeSessionTurn(live.turnId);
    await expect(
      panel
        .locator(`[data-message-id="${live.messageId}"]`)
        .getByTestId("fixture-prompt-history-duration"),
    ).toBeVisible();
    await expect(panel.getByText("Prompt 1", { exact: false }).first()).toBeVisible();
    await apiClient.updateSessionMessage(live.messageId, "fixture updated prompt @daily");
    await panel
      .getByTestId("fixture-prompt-history-row")
      .first()
      .getByRole("button")
      .first()
      .click();
    await expect(panel.getByTestId("fixture-prompt-history-detail")).toContainText(
      "fixture updated prompt",
    );
    const alias = panel.getByTestId("custom-prompt-mention");
    await expect(alias).toHaveAttribute("data-prompt-name", "daily");
    await alias.click();
    await expect(testPage.getByText("Saved daily prompt preview")).toBeVisible();
    await alias.click();

    await panel.getByRole("button", { name: "Open prompt" }).click();
    await expect(session.activeChat().locator(`#msg-${live.messageId}`)).toBeAttached();
    const chatMessage = session.activeChat().locator(`#msg-${live.messageId}`);
    await chatMessage.hover();
    await chatMessage.getByRole("button", { name: "Mark message as favorite" }).click();
    await expect(panel.locator(`[data-message-id="${live.messageId}"]`)).toHaveAttribute(
      "data-favorite",
      "true",
    );

    await apiClient.deleteSessionMessage(older.messageId);
    await expect(panel.locator(`[data-message-id="${older.messageId}"]`)).toHaveCount(0);

    await testPage.evaluate(() => {
      document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
    });
    await testPage.reload();
    await session.waitForLoad();
    await session.addPanelButton().click();
    const pseudoMenuItem = session.addPanelPluginItem(PLUGIN_ID, PANEL_KEY);
    await expect(pseudoMenuItem).toHaveText(/Ƥřǿɱƥŧ ħīşŧǿřẏ ƒīẋŧŭřḗ/);
    await pseudoMenuItem.click();
    await expect(testPage.getByTestId("fixture-prompt-history-panel")).toContainText(/Ƥřǿɱƥŧ/);
    await testPage.evaluate(() => {
      document.cookie = "kandev_locale=en; path=/; max-age=31536000; SameSite=Lax";
    });
    await testPage.reload();
    await session.waitForLoad();
    await session.addPanelButton().click();
    await session.addPanelPluginItem(PLUGIN_ID, PANEL_KEY).click();
    await apiClient.deleteSession(sessionId);
    await expect(testPage.getByTestId("fixture-prompt-history-panel")).toHaveAttribute(
      "data-removed",
      "true",
    );
    await expect(
      testPage
        .getByTestId("fixture-prompt-history-panel")
        .locator(`[data-message-id="${live.messageId}"]`),
    ).toBeVisible();
  });
});
