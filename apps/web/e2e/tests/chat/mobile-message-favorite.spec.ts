import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const SEEDED_MESSAGE = "Mobile favorite touch target fixture message";

test.describe("Mobile chat message favorite toggle", () => {
  test("offers a 44px target and toggles a message by touch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile Message Favorite", {
      description: "mobile favorite touch target fixture",
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
    const messageBody = chat
      .locator("[data-agent-message-body][data-message-id]")
      .filter({ hasText: SEEDED_MESSAGE });
    const star = messageBody
      .locator("xpath=..")
      .getByRole("button", { name: "Mark message as favorite" });
    await expect(star).toBeVisible();

    const box = await star.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.width).toBeGreaterThanOrEqual(44);
    expect(box!.height).toBeGreaterThanOrEqual(44);

    await star.tap();
    await expect(
      messageBody
        .locator("xpath=..")
        .getByRole("button", { name: "Remove message from favorites" }),
    ).toBeVisible();
  });
});
