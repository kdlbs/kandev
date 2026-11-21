import type { Page } from "@playwright/test";

import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { waitForHttp, watchWs } from "../../helpers/causal-waits";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

async function seedAndOpenChat(page: Page, apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Picker Configuration Feedback",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("expected an auto-started picker feedback session");
  await waitForAgentMessage(
    apiClient,
    task.session_id,
    "This is a simple mock response for e2e testing.",
    30_000,
  );
  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "WAITING_FOR_INPUT",
    timeout: 30_000,
    message: "initial picker feedback turn must settle before interaction",
  });
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return { session, sessionId: task.session_id };
}

test.describe("Picker feedback", () => {
  test.describe.configure({ retries: 1, timeout: 90_000 });

  test("reports the effective model and pauses expiry while the picker is open", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedAndOpenChat(testPage, apiClient, seedData);
    await testPage.clock.install();
    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toContainText("Mock Fast");

    await trigger.click();
    const configurationResponse = waitForHttp(
      testPage,
      "POST",
      /\/api\/v1\/task-sessions\/[^/]+\/set-(?:config-option|model|mode)$/,
    );
    await testPage.getByRole("option", { name: /Mock Smart/ }).click();
    await configurationResponse;
    const feedback = testPage.getByTestId("session-configuration-feedback");
    await expect(feedback).toBeVisible();
    await expect(feedback).toHaveAttribute("role", "status");
    await expect(feedback).toHaveAttribute("data-status", "applied");
    await expect(feedback).toContainText("mock-smart");

    const listbox = testPage.getByRole("listbox");
    await expect(listbox).toBeVisible();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await testPage.clock.runFor(1_500);
    await expect(feedback).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(listbox).toBeHidden();
    await trigger.click();
    await expect(listbox).toBeVisible();
    await testPage.keyboard.press("Escape");
    await testPage.clock.runFor(1_000);
    await expect(feedback).toBeHidden();
    await expect(testPage.getByTestId("toast-message")).toHaveCount(0);
    await expect(trigger).toContainText("Mock Smart");
    await expect(session.chat).toContainText("This is a simple mock response for e2e testing.");
  });

  test("shows the observed model under its silent configuration command", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const ws = watchWs(testPage);
    const { session, sessionId } = await seedAndOpenChat(testPage, apiClient, seedData);
    await testPage.clock.install();
    const completion = ws.waitForEvent("session.turn.completed", {
      where: (payload) => payload.session_id === sessionId,
    });
    const conversationChange = ws.waitForEvent("session.conversation.changed", {
      where: (payload) =>
        payload.session_id === sessionId &&
        Array.isArray(payload.operations) &&
        payload.operations.some((value) => {
          if (!value || typeof value !== "object") return false;
          const operation = value as { entity?: unknown; turn?: { completed_at?: unknown } };
          return operation.entity === "turn" && Boolean(operation.turn?.completed_at);
        }),
    });
    await session.sendMessage("/model mock-smart");

    const command = session.chat.getByText("/model mock-smart", { exact: true });
    const receipt = session.chat
      .getByRole("status")
      .filter({ hasText: "The agent applied Mock Smart." });
    const [completedTurn, changed] = await Promise.all([completion, conversationChange]);
    expect(completedTurn.payload.had_output).toBe(false);
    expect(completedTurn.payload.configuration_result).toMatchObject({
      status: "applied",
      config_id: "model",
      requested: "Mock Smart",
      effective: "Mock Smart",
      confirmed: true,
    });
    const operation = (changed.payload.operations as Array<Record<string, unknown>>).find(
      (candidate) =>
        candidate.entity === "turn" &&
        candidate.id === completedTurn.payload.id &&
        Boolean((candidate.turn as { completed_at?: unknown } | undefined)?.completed_at),
    );
    expect(operation).toBeDefined();
    expect(operation?.configuration_result).toMatchObject({
      status: "applied",
      requested: "Mock Smart",
      effective: "Mock Smart",
      confirmed: true,
    });
    await expect(command).toBeVisible();
    await expect(receipt).toHaveCount(1);
    await expect(receipt).toContainText("The agent applied Mock Smart.");
    await expect(session.chat).not.toContainText(/finished without producing any output/i);
    const followsCommand = await receipt.evaluate((status, commandText) => {
      const commandMessage = Array.from(
        document.querySelectorAll('[data-testid="user-message-bubble"]'),
      ).find((message) => message.textContent?.trim() === commandText);
      return (
        commandMessage !== undefined &&
        Boolean(commandMessage.compareDocumentPosition(status) & Node.DOCUMENT_POSITION_FOLLOWING)
      );
    }, "/model mock-smart");
    expect(followsCommand).toBe(true);

    await testPage.clock.runFor(5_000);
    await expect(receipt).toHaveCount(0);
  });
});
