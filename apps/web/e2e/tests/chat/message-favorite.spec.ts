// Regression guard: a session transcript message can be starred as a
// "favorite" so a user can find it again quickly once the session has moved
// on. Starring toggles the star icon's pressed state and tints the message
// background light yellow (see message-actions.tsx / agent-message-content.tsx).
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const SEEDED_MESSAGE = "Favorite toggle regression fixture message";

test.describe("Chat message favorite toggle", () => {
  test("keeps a starred message highlighted after reload and un-stars it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Message Favorite", {
      description: "seeded message favorite fixture",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "message",
      content: SEEDED_MESSAGE,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const chat = session.activeChat();
    await expect(chat.getByText(SEEDED_MESSAGE)).toBeVisible({ timeout: 15_000 });

    // Scope to the seeded message's own row, mirroring the timestamp-tooltip spec.
    const messageBody = chat
      .locator("[data-agent-message-body][data-message-id]")
      .filter({ hasText: SEEDED_MESSAGE });
    const highlight = messageBody.locator("xpath=..");
    const star = highlight.getByRole("button", { name: "Mark message as favorite" });
    await expect(star).toBeVisible();
    expect(await highlight.getAttribute("class")).not.toMatch(/yellow/);

    await star.click();

    const unstar = highlight.getByRole("button", { name: "Remove message from favorites" });
    await expect(unstar).toBeVisible();
    await expect.poll(async () => highlight.getAttribute("class")).toMatch(/yellow/);

    await testPage.reload();
    await session.waitForLoad();

    await expect(chat.getByText(SEEDED_MESSAGE)).toBeVisible();
    await expect(unstar).toBeVisible();
    await expect.poll(async () => highlight.getAttribute("class")).toMatch(/yellow/);

    await unstar.click();

    await expect(star).toBeVisible();
    await expect.poll(async () => highlight.getAttribute("class")).not.toMatch(/yellow/);
  });
});
