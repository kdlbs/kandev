// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { seedIdleSession } from "../../helpers/session";
import { typeWhileBusy } from "../../helpers/type-while-busy";

test.describe("mobile: pause queue recovery", () => {
  test.describe.configure({ retries: 1 });

  test("force-stop parks the next message until Run next is tapped", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Mobile pause parks queue",
    );
    const chat = session.activeChat();

    await session.sendMessageViaButton("/slow 8s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });

    const editor = chat.locator(".tiptap.ProseMirror");
    await typeWhileBusy(testPage, editor, "/e2e:simple-message");
    await chat.getByTestId("submit-message-button").tap();

    const queueChip = chat.getByTestId("queue-chip");
    await expect(queueChip).toBeVisible({ timeout: 10_000 });

    await session.cancelAgentButton().tap();

    await expect(session.agentStatus()).not.toBeVisible({ timeout: 30_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(queueChip).toBeVisible();
    await expect(chat.getByText("simple mock response", { exact: false }).nth(1)).not.toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);

    await queueChip.tap();
    const runNext = chat.getByTestId("queue-drain-next");
    await expect(runNext).toBeVisible();
    await expect(runNext).toBeInViewport();
    await runNext.tap();

    await expect(chat.getByTestId("queue-chip")).not.toBeVisible({ timeout: 30_000 });
    await session.expectChatResponseVisible("simple mock response", 1, { timeout: 30_000 });
  });
});
