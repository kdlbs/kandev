// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  waitForActiveSessionCancellationPending,
  waitForActiveSessionCancellationPendingOrSettled,
  waitForActiveSessionForegroundActivity,
} from "../../helpers/session-store";
import { seedIdleSession } from "../../helpers/session";

test.describe.serial("Mobile cancel turn availability", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({ KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF: "true" });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("keeps the background cancel target touch-sized and reachable", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Mobile background cancellation availability",
    );

    await session.sendMessageViaButton("/detached-background 20s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 20_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await waitForActiveSessionForegroundActivity(testPage, "background");

    const chat = session.activeChat();
    const cancel = chat.getByTestId("cancel-agent-button");
    await expect(cancel).toBeVisible();
    const [cancelBox, composerBox] = await Promise.all([
      cancel.boundingBox(),
      chat.getByTestId("chat-input-area").boundingBox(),
    ]);
    expect(cancelBox).not.toBeNull();
    expect(composerBox).not.toBeNull();
    expect(cancelBox!.width).toBeGreaterThanOrEqual(44);
    expect(cancelBox!.height).toBeGreaterThanOrEqual(44);
    expect(cancelBox!.y).toBeGreaterThanOrEqual(composerBox!.y);
    expect(cancelBox!.y + cancelBox!.height).toBeLessThanOrEqual(
      composerBox!.y + composerBox!.height + 1,
    );

    await prCapture.screenshot("mobile-cancel-turn-availability", {
      caption: "Mobile background work keeps the cancel control reachable in the composer",
    });

    await cancel.tap();
    await waitForActiveSessionCancellationPendingOrSettled(testPage);
    await expect
      .poll(async () => {
        if (!(await cancel.isVisible().catch(() => false))) return true;
        return cancel.isDisabled();
      })
      .toBe(true);
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await waitForActiveSessionCancellationPending(testPage, false);
    await waitForActiveSessionForegroundActivity(testPage, null);
    await expect(cancel).not.toBeVisible({ timeout: 15_000 });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile background cancellation");
  });
});
