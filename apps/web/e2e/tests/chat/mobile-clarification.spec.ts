import { test, expect } from "../../fixtures/test-base";
import { seedClarificationSession } from "../../helpers/clarification";

/**
 * Mobile parity for the multiline custom clarification answer. On a coarse-pointer
 * device Enter inserts a newline instead of submitting, and the send affordance is
 * the inline "Send" button (there is no overlay Submit button for single-question
 * bundles). Runs under the Pixel 5 `mobile-chrome` project.
 */
test.describe("Mobile clarification multiline answer", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("queues inline composer input while the structured question remains answerable", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Clarify Composer Queue",
      { scenario: "clarification" },
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    const composer = session.activeChat().getByTestId("chat-input-editor");
    await expect(composer).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
    await composer.fill("Queue this from phone", { timeout: 30_000 });
    await testPage.getByTestId("submit-message-button").tap();

    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });
    await expect(session.clarificationOverlay()).toBeVisible();
    await testPage.getByTestId("queue-chip").tap();
    await expect(testPage.getByTestId("queued-ghost-list")).toBeVisible();
    await expect(testPage.getByTestId("queue-drain-next")).not.toBeVisible();
  });

  test("Enter inserts a newline and the Send button submits the multiline answer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Clarify",
      {
        scenario: "clarification",
      },
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    const input = session.clarificationInput();
    // Tap the apparent row surface, outside the textarea itself. The whole row
    // is the mobile touch target and should transfer focus into the textarea.
    await session.clarificationCustomInput().tap({ position: { x: 4, y: 4 } });
    await expect(input).toBeFocused();
    await input.pressSequentially("first line");
    // On touch, Enter inserts a newline rather than submitting.
    await input.press("Enter");
    await input.pressSequentially("second line");
    await expect(input).toHaveValue("first line\nsecond line");
    // The overlay is still open — Enter did not submit.
    await expect(session.clarificationOverlay()).toBeVisible();

    // The inline Send button is the touch send affordance.
    await expect(session.clarificationCustomSubmit()).toBeVisible();
    await session.clarificationCustomSubmit().tap();

    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("first line");
    await expect(session.chat).toContainText("second line");
    await expect(session.chat).not.toContainText("linesecond line");
  });
});
