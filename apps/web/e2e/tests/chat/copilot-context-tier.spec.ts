import { test, expect } from "../../fixtures/test-base";
import { copilotProfileSaveActions } from "../../helpers/copilot-plugin-fixture";
import { mockTurnGate } from "../../helpers/mock-turn-gate";
import { waitForSessionState } from "../../helpers/session";
import {
  createDisposableCopilotProfile,
  enableCopilotNativeBackend,
  startCopilotContextTask,
  waitForCopilotContextInfoMessages,
} from "./copilot-context-tier-helpers";

test.describe("Copilot context window selection", () => {
  test.setTimeout(120_000);

  test("configures Copilot profile context window, previews --context flag, and launches with long_context", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const release = await enableCopilotNativeBackend(backend);
    let profile: Awaited<ReturnType<typeof createDisposableCopilotProfile>> | null = null;
    try {
      profile = await createDisposableCopilotProfile(apiClient);
      await testPage.goto(`/settings/agents/${profile.agentName}/profiles/${profile.profileId}`);

      const refresh = testPage.getByTestId("profile-refresh-capabilities");
      await expect(refresh).toBeEnabled({ timeout: 20_000 });
      await refresh.click();
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 30_000 },
      );

      const selector = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(selector).toBeVisible({ timeout: 15_000 });
      await selector.click();

      const contextTrigger = testPage.getByTestId("config-option-trigger-context_tier");
      await expect(contextTrigger).toBeVisible({ timeout: 10_000 });
      await expect(contextTrigger).toContainText("Context Window");
      await expect(contextTrigger).toContainText("Default");
      await contextTrigger.click();

      const contextSection = testPage.getByTestId("config-option-section-context_tier");
      await expect(contextSection).toBeVisible();
      await contextSection.getByRole("button", { name: "Long Context", exact: true }).click();
      await expect(selector).toContainText("Long Context");
      await testPage.keyboard.press("Escape");

      const previewCode = testPage.locator('[data-settings-target$="-command-preview"] pre code');
      await expect(previewCode).toHaveCount(1);
      await expect(previewCode).toContainText("--context long_context", { timeout: 15_000 });

      const saveActions = copilotProfileSaveActions(testPage);
      await expect(saveActions.save).toBeEnabled({ timeout: 10_000 });
      await saveActions.save.click();
      await expect
        .poll(async () => (await apiClient.getAgentProfile(profile!.profileId)).configOptions)
        .toMatchObject({ context_tier: "long_context" });

      await testPage.reload();
      await expect(selector).toContainText("Long Context", { timeout: 15_000 });
      await expect(
        testPage.locator('[data-settings-target$="-command-preview"] pre code'),
      ).toContainText("--context long_context", { timeout: 15_000 });

      const { sessionId, session } = await startCopilotContextTask(
        testPage,
        apiClient,
        seedData,
        profile.profileId,
      );
      const [firstTurn] = await waitForCopilotContextInfoMessages(apiClient, sessionId, 1);
      expect(firstTurn).toMatchObject({
        contextTier: "long_context",
        loads: 0,
        priorTurns: 0,
      });
      await expect(session.chat).toContainText("Copilot runtime context_tier=long_context");
    } finally {
      if (profile) await profile.dispose();
      await release();
    }
  });

  test("reloads an idle Copilot session into long_context while preserving conversation history", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const release = await enableCopilotNativeBackend(backend);
    let profile: Awaited<ReturnType<typeof createDisposableCopilotProfile>> | null = null;
    try {
      profile = await createDisposableCopilotProfile(apiClient);
      const { taskId, sessionId, session } = await startCopilotContextTask(
        testPage,
        apiClient,
        seedData,
        profile.profileId,
      );

      const [initialInfo] = await waitForCopilotContextInfoMessages(apiClient, sessionId, 1);
      expect(initialInfo).toMatchObject({
        contextTier: "default",
        loads: 0,
        priorTurns: 0,
      });
      await expect(session.chat).toContainText("Copilot runtime context_tier=default");

      const trigger = testPage.getByRole("button", { name: "Session model settings" });
      await expect(trigger).toBeVisible({ timeout: 15_000 });
      await trigger.click();

      const contextTrigger = testPage.getByTestId("config-option-trigger-context_tier");
      await expect(contextTrigger).toBeVisible({ timeout: 10_000 });
      await contextTrigger.click();

      const contextSection = testPage.getByTestId("config-option-section-context_tier");
      await expect(contextSection).toBeVisible();
      await contextSection.getByRole("button", { name: "Long Context", exact: true }).click();
      await expect(trigger).toContainText("Long Context", { timeout: 15_000 });
      await testPage.keyboard.press("Escape");

      await expect
        .poll(async () => {
          const { sessions } = await apiClient.listTaskSessionDetails(taskId);
          const runtime = sessions.find((item) => item.id === sessionId)?.metadata
            ?.runtime_config as { config_options?: Record<string, string> } | undefined;
          return runtime?.config_options?.context_tier;
        })
        .toBe("long_context");

      await session.sendMessage("/e2e:copilot-context-info second-turn");
      const turns = await waitForCopilotContextInfoMessages(apiClient, sessionId, 2);
      await waitForSessionState(apiClient, {
        taskId,
        sessionId,
        expectedState: "WAITING_FOR_INPUT",
        timeout: 30_000,
        message: "Reloaded Copilot session must complete the follow-up turn",
      });
      await session.waitForChatIdle({ timeout: 30_000 });

      expect(turns[1].contextTier).toBe("long_context");
      expect(turns[1].sessionId).toBe(initialInfo.sessionId);
      expect(turns[1].pid).not.toBe(initialInfo.pid);
      expect(turns[1].loads).toBe(1);
      expect(turns[1].priorTurns).toBe(1);

      await expect(session.chat).toContainText("Copilot runtime context_tier=default");
      await expect(session.chat).toContainText("Copilot runtime context_tier=long_context");

      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle({ timeout: 30_000 });
      await expect(trigger).toContainText("Long Context", { timeout: 15_000 });
      await expect(session.chat).toContainText("Copilot runtime context_tier=default");
      await expect(session.chat).toContainText("Copilot runtime context_tier=long_context");
    } finally {
      if (profile) await profile.dispose();
      await release();
    }
  });

  test("rejects live context window switch during an active turn without interrupting it, then applies once idle", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const release = await enableCopilotNativeBackend(backend);
    let profile: Awaited<ReturnType<typeof createDisposableCopilotProfile>> | null = null;
    const gate = mockTurnGate(backend.tmpDir, "Active Copilot turn finished");
    try {
      profile = await createDisposableCopilotProfile(apiClient);
      const { taskId, sessionId, session } = await startCopilotContextTask(
        testPage,
        apiClient,
        seedData,
        profile.profileId,
      );
      const [initialInfo] = await waitForCopilotContextInfoMessages(apiClient, sessionId, 1);
      expect(initialInfo.contextTier).toBe("default");

      await session.sendMessage(gate.prompt);
      await expect(
        session.activeChat().getByText("Queue blocking turn started", { exact: true }),
      ).toBeVisible({ timeout: 20_000 });
      await waitForSessionState(apiClient, {
        taskId,
        sessionId,
        expectedState: "RUNNING",
        timeout: 15_000,
        message: "Copilot turn must remain active before attempting context_tier switch",
      });

      const trigger = testPage.getByRole("button", { name: "Session model settings" });
      await expect(trigger).toBeVisible({ timeout: 15_000 });
      await trigger.click();

      const contextTrigger = testPage.getByTestId("config-option-trigger-context_tier");
      await expect(contextTrigger).toBeVisible({ timeout: 10_000 });
      await contextTrigger.click();

      const rejectedResponse = testPage.waitForResponse(
        (res) =>
          res.url().includes(`/api/v1/task-sessions/${sessionId}/set-config-option`) &&
          res.request().method() === "POST",
      );
      await testPage
        .getByTestId("config-option-section-context_tier")
        .getByRole("button", { name: "Long Context", exact: true })
        .click();
      const rejected = await rejectedResponse;
      expect(rejected.ok()).toBe(false);
      const rejectedBody = (await rejected.json()) as {
        configuration_result?: { status?: string; confirmed?: boolean };
      };
      expect(rejectedBody.configuration_result).toMatchObject({
        status: "rejected",
        confirmed: false,
      });
      await testPage.keyboard.press("Escape");

      const feedback = session.activeChat().getByTestId("session-configuration-feedback");
      await expect(feedback).toHaveCount(1);
      await expect(feedback).toContainText("Could not apply Context Window: Long Context.");
      await expect(feedback).toContainText("cannot change Copilot context length");
      await expect(trigger).not.toContainText("Long Context");

      const { sessions: midTurnSessions } = await apiClient.listTaskSessionDetails(taskId);
      const midTurnRuntime = midTurnSessions.find((item) => item.id === sessionId)?.metadata
        ?.runtime_config as { config_options?: Record<string, string> } | undefined;
      expect(midTurnRuntime?.config_options?.context_tier).toBe("default");

      await session.activeChat().getByTestId("session-configuration-feedback-dismiss").click();
      await expect(feedback).toHaveCount(0);

      await gate.release();
      await expect(
        session.activeChat().getByText("Active Copilot turn finished", { exact: true }),
      ).toBeVisible({ timeout: 20_000 });
      await waitForSessionState(apiClient, {
        taskId,
        sessionId,
        expectedState: "WAITING_FOR_INPUT",
        timeout: 30_000,
        message: "Active Copilot turn must finish undisturbed after rejected context switch",
      });
      await session.waitForChatIdle({ timeout: 30_000 });

      await trigger.click();
      await testPage.getByTestId("config-option-trigger-context_tier").click();
      await testPage
        .getByTestId("config-option-section-context_tier")
        .getByRole("button", { name: "Long Context", exact: true })
        .click();
      await expect(trigger).toContainText("Long Context", { timeout: 15_000 });
      await testPage.keyboard.press("Escape");

      await session.sendMessage("/e2e:copilot-context-info post-busy-turn");
      const [, reloadedInfo] = await waitForCopilotContextInfoMessages(apiClient, sessionId, 2);
      await waitForSessionState(apiClient, {
        taskId,
        sessionId,
        expectedState: "WAITING_FOR_INPUT",
        timeout: 30_000,
        message: "Post-reload Copilot turn must settle back to WAITING_FOR_INPUT",
      });
      await session.waitForChatIdle({ timeout: 30_000 });
      expect(reloadedInfo.pid).not.toBe(initialInfo.pid);
      expect(reloadedInfo.sessionId).toBe(initialInfo.sessionId);
      expect(reloadedInfo).toMatchObject({
        contextTier: "long_context",
        loads: 1,
        priorTurns: 2,
      });
    } finally {
      await gate.release().catch(() => undefined);
      if (profile) await profile.dispose();
      await release();
    }
  });
});
