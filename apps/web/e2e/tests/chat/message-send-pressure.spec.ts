import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { routeMainWebSocketWithMessageAddResponseDrop } from "../../helpers/ws-drop";
import { openTaskSession } from "../../helpers/session";

test.describe("Message send under notification pressure", () => {
  test("reconciles a lost response without duplicating the prompt or turn", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const wsDrop = await routeMainWebSocketWithMessageAddResponseDrop(testPage);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Message send pressure",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    const session = await openTaskSession(testPage, task.id);
    await session.waitForChatIdle({ timeout: 45_000 });

    const modelSelector = testPage.getByRole("button", { name: "Session model settings" });
    await expect(modelSelector).toBeVisible({ timeout: 15_000 });

    const beforeMessages = await apiClient.listSessionMessages(task.session_id);
    const beforeTurns = await apiClient.listSessionTurns(task.session_id);

    // These events are deliberately irrelevant to the task summary, but they
    // exercise the selected session's stream while the task-level sidebar
    // remains bounded.
    await apiClient.seedToolCallMessages(task.session_id, 80);
    const pressuredMessages = await apiClient.listSessionMessages(task.session_id);
    expect(
      pressuredMessages.messages.filter((message) =>
        message.content.startsWith("synthetic tool call "),
      ),
    ).toHaveLength(80);
    await expect(modelSelector).toBeVisible();

    const prompt = "/slow 8s";
    wsDrop.dropNextMessageAddResponse();
    await session.sendMessageViaButton(prompt);

    await expect
      .poll(wsDrop.droppedCount, {
        message: "expected the test proxy to drop the correlated message.add response",
        timeout: 20_000,
      })
      .toBeGreaterThan(0);
    await expect(
      session.chat.locator(".chat-message-list:visible").getByText(prompt, { exact: false }),
    ).toBeVisible({ timeout: 20_000 });
    await session.waitForChatIdle({ timeout: 45_000 });
    await expect(modelSelector).toBeVisible();

    const afterMessages = await apiClient.listSessionMessages(task.session_id);
    const afterTurns = await apiClient.listSessionTurns(task.session_id);
    expect(
      afterMessages.messages.filter(
        (message) => message.author_type === "user" && message.content === prompt,
      ),
    ).toHaveLength(1);
    expect(afterMessages.messages.length).toBeGreaterThanOrEqual(
      beforeMessages.messages.length + 1,
    );
    expect(afterTurns.turns).toHaveLength(beforeTurns.turns.length + 1);
  });
});
