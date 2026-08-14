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

  test("keeps a pending question open while typing a digit in the inline composer", async ({
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
    await composer.pressSequentially("Queue this from phone 1", { timeout: 30_000 });
    await expect(composer).toContainText("Queue this from phone 1");
    await expect(session.clarificationOverlay()).toBeVisible();
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

  test("shows shared context once above the question on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Clarify Shared Context",
      { scenario: "clarification-multi" },
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    const context = session.clarificationContext();
    await expect(context).toHaveCount(1);
    await expect(context).toHaveText(
      "Picking the foundational stack: answer all three so we can move forward.",
    );
    await expect(context).toHaveCSS("margin-top", "12px");
    await expect(context).toHaveCSS("padding", "0px");
    await expect(context).toHaveCSS("border-width", "0px");
    await expect(context).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    await expect(
      session.clarificationQuestionCards().getByTestId("clarification-context"),
    ).toHaveCount(0);

    const [contextBox, overlayBox] = await Promise.all([
      context.boundingBox(),
      session.clarificationOverlay().boundingBox(),
    ]);
    if (!contextBox || !overlayBox) {
      throw new Error("expected mobile shared context and overlay to have bounding boxes");
    }
    expect(contextBox.x).toBeGreaterThanOrEqual(overlayBox.x - 1);
    expect(contextBox.x + contextBox.width).toBeLessThanOrEqual(
      overlayBox.x + overlayBox.width + 1,
    );
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    await session.clarificationStep(1).tap();
    await expect(session.clarificationStep(1)).toHaveAttribute("data-active", "true");
    await expect(context).toHaveCount(1);
  });
});
