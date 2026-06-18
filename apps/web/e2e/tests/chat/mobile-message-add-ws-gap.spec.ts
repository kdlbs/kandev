import { test, expect } from "../../fixtures/test-base";
import { routeMainWebSocketWithPromptDrop } from "../../helpers/ws-drop";
import { seedIdleSession } from "../../helpers/session";

test.describe("Mobile chat send websocket gaps", () => {
  test("shows an accepted user prompt when its message-added notification is missed", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const wsDrop = await routeMainWebSocketWithPromptDrop(testPage);
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Message Added WS Gap",
    );

    const prompt = "/slow 8s";
    wsDrop.dropPrompt(prompt);

    await session.sendMessageViaButton(prompt);

    await expect(
      session.chat.locator(".chat-message-list:visible").getByText(prompt, { exact: false }),
    ).toBeVisible({ timeout: 5_000 });
    await expect
      .poll(wsDrop.droppedCount, {
        message: "expected the test proxy to drop the prompt's session.message.added frame",
        timeout: 10_000,
      })
      .toBeGreaterThan(0);
  });
});
