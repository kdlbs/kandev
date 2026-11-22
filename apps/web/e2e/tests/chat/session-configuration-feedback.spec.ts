import { test, expect } from "../../fixtures/test-base";
import { waitForHttp, watchWs } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import { seedAndOpenChat } from "./session-configuration-feedback-helpers";

test.describe("Picker feedback", () => {
  test.describe.configure({ retries: 1, timeout: 90_000 });

  // @covers AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.2, AC-AGENTS-SESSION-CONFIG-FEEDBACK-001.7
  test("starts a new feedback row after the first launch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "First launch feedback", {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
      prepare_session: true,
    });
    expect(task.session_id).toBeTruthy();
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const feedback = session.activeChat().getByTestId("session-configuration-feedback");

    await testPage.getByTestId("session-mode-selector").click();
    const saved = waitForHttp(testPage, "POST", /\/set-mode$/);
    await testPage.getByRole("menuitem", { name: /Plan Mock/ }).click();
    expect(await (await saved).json()).toMatchObject({
      configuration_result: { status: "saved_for_launch" },
    });
    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    await trigger.click();
    const savedModel = waitForHttp(testPage, "POST", /\/set-(?:config-option|model)$/);
    await testPage.getByRole("option", { name: /Mock Smart/ }).click();
    expect(await (await savedModel).json()).toMatchObject({
      configuration_result: { status: "saved_for_launch" },
    });
    const savedSettings = "Saved mode Plan Mock, model Mock Smart for agent startup.";
    await expect(feedback).toHaveCount(1);
    await expect(feedback).toHaveText(savedSettings);
    await expect(feedback.locator("code")).toHaveText(["Plan Mock", "Mock Smart"]);
    await testPage.keyboard.press("Escape");

    await testPage.getByTestId("task-description-start-button").click();
    await expect(
      session.activeChat().getByText("This is a simple mock response for e2e testing."),
    ).toBeVisible({ timeout: 30_000 });
    await expect(session.anyIdleInput()).toBeVisible();
    await expect(trigger).toContainText("Mock Smart");
    await expect(testPage.getByTestId("session-mode-selector")).toContainText("Plan Mock");
    await trigger.click();
    const applied = waitForHttp(testPage, "POST", /\/set-(?:config-option|model)$/);
    await testPage.getByRole("option", { name: /Mock Slow/ }).click();
    expect(await (await applied).json()).toMatchObject({
      configuration_result: { status: "applied", confirmed: true },
    });

    await expect(
      session
        .activeChat()
        .locator(
          '[data-testid="session-configuration-feedback"], [data-testid="user-message-bubble"]',
        ),
    ).toHaveText([savedSettings, "/e2e:simple-message", "Changed model Mock Slow."]);
  });

  test("merges picker changes before the next turn in one muted transcript row", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedAndOpenChat(
      testPage,
      apiClient,
      seedData,
      "Picker Configuration Feedback",
    );
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
    const transcript = session.activeChat().locator(".chat-message-list");
    const feedback = transcript.getByTestId("session-configuration-feedback");
    await expect(feedback).toBeVisible();
    await expect(feedback).toHaveAttribute("role", "status");
    await expect(feedback).toHaveClass(/text-muted-foreground/);
    await expect(feedback).toHaveText("Changed model Mock Smart.");
    await expect(feedback.locator("code")).toHaveText(["Mock Smart"]);

    await testPage.keyboard.press("Escape");
    await testPage.getByTestId("session-mode-selector").click();
    const modeResponse = waitForHttp(testPage, "POST", /\/set-mode$/);
    await testPage.getByRole("menuitem", { name: /Plan Mock/ }).click();
    await modeResponse;
    await expect(feedback).toHaveCount(1);
    await expect(feedback).toHaveText("Changed model Mock Smart, mode Plan Mock.");

    await testPage.clock.runFor(6_000);
    await trigger.click();
    await testPage.getByTestId("config-option-trigger-effort").click();
    const effortResponse = waitForHttp(testPage, "POST", /\/set-config-option$/);
    await testPage.getByRole("button", { name: "Low", exact: true }).click();
    await effortResponse;
    await expect(feedback).toHaveCount(1);
    await expect(feedback).toHaveText("Changed model Mock Smart, mode Plan Mock, effort Low.");
    await expect(feedback.locator("code")).toHaveText(["Mock Smart", "Plan Mock", "Low"]);
    await testPage.keyboard.press("Escape");

    const prompt = "use this configuration";
    await session.sendMessage(prompt);
    await session.expectChatResponseVisible(prompt);
    await session.waitForChatIdle({ timeout: 30_000 });
    await trigger.click();
    const nextModelResponse = waitForHttp(testPage, "POST", /\/set-config-option$/);
    await testPage.getByRole("option", { name: /Mock Fast/ }).click();
    await nextModelResponse;
    await expect(feedback).toHaveCount(2);
    await expect(feedback.first()).toHaveText(
      "Changed model Mock Smart, mode Plan Mock, effort Low.",
    );
    await expect(feedback.last()).toHaveText("Changed model Mock Fast.");
    await expect(testPage.getByTestId("toast-message")).toHaveCount(0);

    await testPage.reload();
    await session.waitForLoad();
    await expect(trigger).toContainText("Mock Fast");
    await expect(feedback).toHaveCount(0);
  });

  test("shows the observed model under its silent configuration command", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const ws = watchWs(testPage);
    const { session, sessionId } = await seedAndOpenChat(
      testPage,
      apiClient,
      seedData,
      "Picker Command Feedback",
    );
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
    await expect(receipt).toHaveClass(/text-muted-foreground/);
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
