import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const SEEDED_MESSAGE = "Mobile favorite touch target fixture message";

test.describe("Mobile chat message favorite toggle", () => {
  test("matches sibling action icon size and toggles a message by touch", async ({
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
    const actionsRow = messageBody.locator("xpath=..");
    const star = actionsRow.getByRole("button", { name: "Mark message as favorite" });
    const copy = actionsRow.getByRole("button", { name: "Copy message to clipboard" });
    await expect(star).toBeVisible();
    await expect(copy).toBeVisible();

    // The star must render at the same size as the sibling action icons on
    // the phone viewport instead of the oversized 44px target.
    const starBox = await star.boundingBox();
    const copyBox = await copy.boundingBox();
    expect(starBox).not.toBeNull();
    expect(copyBox).not.toBeNull();
    expect(Math.abs(starBox!.width - copyBox!.width)).toBeLessThanOrEqual(2);
    expect(Math.abs(starBox!.height - copyBox!.height)).toBeLessThanOrEqual(2);

    await star.tap();
    await expect(
      actionsRow.getByRole("button", { name: "Remove message from favorites" }),
    ).toBeVisible();
  });
});
