import { test, expect } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import {
  openConfigurationChat,
  confirmConfigurationChatRestart,
  sendConfigurationPrompt,
  waitForConfigurationResponse,
  openUncertainExpandedConfigChat,
} from "../../helpers/config-chat-restart";

test.describe("Configuration Chat restart", () => {
  test.beforeEach(async ({ apiClient, seedData }) => {
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_config_agent_profile_id: seedData.agentProfileId,
    });
  });

  test("offers recovery in the header and disables it before a session exists", async ({
    testPage,
  }) => {
    const panel = await openConfigurationChat(testPage);
    const restart = panel.getByRole("button", { name: "Restart session", exact: true });
    await expect(restart).toBeDisabled();
    await expect.poll(async () => (await restart.boundingBox())?.height).toBe(28);
  });

  test("refreshes an uncertain restart from expanded chat without another restart", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const capture = new PrAssetCapture(testPage, "config-chat-restart.spec.ts", {
      captureKey: "expanded",
    });
    const old = await apiClient.startConfigChat(
      seedData.workspaceId,
      seedData.agentProfileId,
      'e2e:message("expanded original response")',
    );
    await waitForConfigurationResponse(apiClient, old.session_id, "expanded original response");
    let restartRequests = 0;
    testPage.on("request", (request) => {
      if (request.method() === "POST" && request.url().endsWith("/config-chat/restart"))
        restartRequests++;
    });
    const dialog = await openUncertainExpandedConfigChat(
      testPage,
      seedData.workspaceId,
      old.session_id,
    );
    const refresh = dialog.getByRole("button", { name: "Refresh status", exact: true });
    await expect(refresh).toHaveCount(1);
    await capture.screenshot("desktop-expanded-restart-recovery", {
      caption: "Expanded Configuration Chat can refresh an uncertain restart",
    });
    capture.flush();
    expect(
      await refresh.evaluate((element) => {
        const rect = element.getBoundingClientRect();
        return element.contains(
          document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2),
        );
      }),
    ).toBe(true);
    await refresh.click();
    await expect(dialog.getByText("expanded original response", { exact: true })).toBeVisible();
    await expect(dialog.getByRole("button", { name: "Refresh status", exact: true })).toHaveCount(
      0,
    );
    expect((await apiClient.getTaskSession(old.session_id)).session.id).toBe(old.session_id);
    expect(restartRequests).toBe(0);
  });

  test("cancels without losing a draft, then replaces and restores a blank conversation", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const old = await apiClient.startConfigChat(
      seedData.workspaceId,
      seedData.agentProfileId,
      'e2e:message("old configuration response")',
    );
    const panel = await openConfigurationChat(testPage);
    await waitForConfigurationResponse(apiClient, old.session_id, "old configuration response");
    await expect(panel.getByText("old configuration response", { exact: true })).toBeVisible();
    const editor = panel.getByTestId("chat-input-editor");
    await editor.fill("unsent configuration prompt");
    await panel.getByRole("button", { name: "Restart session", exact: true }).click();
    const confirmation = testPage.getByTestId("config-chat-restart-confirmation");
    await expect(confirmation).toBeVisible();
    await expect(confirmation).toHaveCSS("opacity", "1");
    await prCapture.screenshot("desktop-restart-confirmation", {
      caption: "Configuration Chat restart confirmation in the header",
    });
    await confirmation.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(editor).toContainText("unsent configuration prompt");
    const replacement = await confirmConfigurationChatRestart(testPage, panel);
    expect(replacement.task_id).not.toBe(old.task_id);
    expect(replacement.session_id).not.toBe(old.session_id);
    expect(replacement.agent_profile_id).toBe(seedData.agentProfileId);
    expect((await apiClient.rawRequest("GET", `/api/v1/tasks/${old.task_id}`)).status).toBe(404);
    expect(
      (await apiClient.rawRequest("GET", `/api/v1/task-sessions/${old.session_id}`)).status,
    ).toBe(404);
    await expect(panel.getByText("old configuration response", { exact: true })).toHaveCount(0);
    await expect(editor).toBeEmpty({ timeout: 15_000 });
    await expect
      .poll(async () => (await apiClient.getTaskSession(replacement.session_id)).session.state)
      .toBe("WAITING_FOR_INPUT");
    const { turns } = await apiClient.listSessionTurns(replacement.session_id);
    expect(turns.filter((turn) => turn.metadata?.lifecycle_only !== true)).toEqual([]);
    expect(turns.every((turn) => Boolean(turn.completed_at))).toBe(true);
    const { messages } = await apiClient.listSessionMessages(replacement.session_id);
    expect(
      messages.filter(
        (message) =>
          ["message", "content", "thinking"].includes(message.type ?? "") ||
          message.type?.startsWith("tool_"),
      ),
    ).toEqual([]);
    await sendConfigurationPrompt(
      panel,
      'e2e:message("after restart response")',
      apiClient,
      replacement.session_id,
    );
    await waitForConfigurationResponse(apiClient, replacement.session_id, "after restart response");
    await expect(panel.getByText("after restart response", { exact: true })).toBeVisible();
    await panel.getByRole("button", { name: "Open in Quick Chat" }).click();
    const expanded = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(expanded.getByText("after restart response", { exact: true })).toBeVisible();
    await expect(expanded.getByText("old configuration response", { exact: true })).toHaveCount(0);
    await testPage.reload();
    await testPage.getByRole("button", { name: "Configuration Chat", exact: true }).click();
    await expect(panel.getByText("after restart response", { exact: true })).toBeVisible();
    await expect(panel.getByText("old configuration response", { exact: true })).toHaveCount(0);
  });

  test("waits for a running agent to stop and explicitly starts with auto-start disabled", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
    try {
      const old = await apiClient.startConfigChat(
        seedData.workspaceId,
        seedData.agentProfileId,
        "/slow 30s",
      );
      await expect
        .poll(async () => (await apiClient.getTaskSession(old.session_id)).session.state, {
          timeout: 30_000,
        })
        .toBe("RUNNING");
      const panel = await openConfigurationChat(testPage);
      const replacement = await confirmConfigurationChatRestart(testPage, panel);
      await expect
        .poll(async () => (await apiClient.getTaskSession(replacement.session_id)).session.state, {
          timeout: 30_000,
        })
        .toBe("WAITING_FOR_INPUT");
      await sendConfigurationPrompt(
        panel,
        'e2e:message("active restart response")',
        apiClient,
        replacement.session_id,
      );
      await waitForConfigurationResponse(
        apiClient,
        replacement.session_id,
        "active restart response",
      );
      await expect(panel.getByText("active restart response", { exact: true })).toBeVisible();
    } finally {
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: false });
    }
  });

  test("sizes fine-pointer controls at the phone boundary and cancels a resized confirmation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const old = await apiClient.startConfigChat(
      seedData.workspaceId,
      seedData.agentProfileId,
      'e2e:message("resize response")',
    );
    await waitForConfigurationResponse(apiClient, old.session_id, "resize response");
    const panel = await openConfigurationChat(testPage);
    await expect(panel.getByTestId("chat-input-editor")).toBeVisible();
    await testPage.setViewportSize({ width: 767, height: 900 });
    for (const label of ["Restart session", "Open in Quick Chat", "Close configuration chat"]) {
      const control = panel.getByRole("button", { name: label, exact: true });
      await expect.poll(async () => (await control.boundingBox())?.height).toBe(44);
    }
    await panel.getByRole("button", { name: "Restart session", exact: true }).click();
    await expect(testPage.getByTestId("config-chat-restart-confirmation")).toBeVisible();
    await testPage.setViewportSize({ width: 768, height: 900 });
    await expect(testPage.getByTestId("config-chat-restart-confirmation")).toHaveCount(0);
    await expect
      .poll(
        async () =>
          (await panel.getByRole("button", { name: "Restart session", exact: true }).boundingBox())
            ?.height,
      )
      .toBe(28);
    await expect(panel.getByText("resize response", { exact: true })).toBeVisible();
  });

  test("replaces a passthrough session with a fresh terminal using the same profile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { agents } = await apiClient.listAgents();
    const owner = agents.find((agent) =>
      agent.profiles?.some((profile) => profile.id === seedData.agentProfileId),
    );
    expect(owner).toBeDefined();
    const profile = await apiClient.createAgentProfile(owner!.id, "Config restart terminal", {
      model: "mock-fast",
      cli_passthrough: true,
    });
    try {
      await apiClient.updateWorkspace(seedData.workspaceId, {
        default_config_agent_profile_id: profile.id,
      });
      const old = await apiClient.startConfigChat(seedData.workspaceId, profile.id);
      const panel = await openConfigurationChat(testPage);
      const terminal = panel.getByTestId("passthrough-terminal");
      await expect(terminal).toBeVisible({ timeout: 15_000 });
      await expect(terminal.getByTestId("passthrough-loading")).not.toBeVisible({
        timeout: 30_000,
      });
      const replacement = await confirmConfigurationChatRestart(testPage, panel);
      expect(replacement.session_id).not.toBe(old.session_id);
      expect(replacement.agent_profile_id).toBe(profile.id);
      await expect(terminal).toBeVisible({ timeout: 15_000 });
      await expect(terminal.getByTestId("passthrough-loading")).not.toBeVisible({
        timeout: 30_000,
      });
      await terminal.click();
      await expect(terminal.locator(".xterm-helper-textarea")).toBeFocused();
    } finally {
      await apiClient.updateWorkspace(seedData.workspaceId, {
        default_config_agent_profile_id: seedData.agentProfileId,
      });
      await apiClient.cleanupTestProfiles([profile.id]);
    }
  });

  test("shows a stop error with the old transcript and allows a successful retry", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const old = await apiClient.startConfigChat(
      seedData.workspaceId,
      seedData.agentProfileId,
      'e2e:message("preserved conversation")',
    );
    const panel = await openConfigurationChat(testPage);
    await waitForConfigurationResponse(apiClient, old.session_id, "preserved conversation");
    await expect(panel.getByText("preserved conversation", { exact: true })).toBeVisible();
    await testPage.route("**/config-chat/restart", async (route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({
          code: "config_chat_restart_stop_failed",
          stage: "stop",
          old_deleted: false,
        }),
      }),
    );
    await panel.getByRole("button", { name: "Restart session", exact: true }).click();
    await testPage.getByTestId("config-chat-confirm-restart").click();
    await expect(panel.getByRole("alert")).toContainText("Could not stop the current agent.");
    await expect(panel.getByText("preserved conversation", { exact: true })).toBeVisible();
    await testPage.unroute("**/config-chat/restart");
    const replacement = await confirmConfigurationChatRestart(testPage, panel);
    await expect(panel.getByRole("alert")).toHaveCount(0);
    await expect(panel.getByText("preserved conversation", { exact: true })).toHaveCount(0);
    await sendConfigurationPrompt(
      panel,
      'e2e:message("retry response")',
      apiClient,
      replacement.session_id,
    );
    await waitForConfigurationResponse(apiClient, replacement.session_id, "retry response");
    await expect(panel.getByText("retry response", { exact: true })).toBeVisible();
  });
});
