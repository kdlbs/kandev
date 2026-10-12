// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  waitForActiveSessionCancellationPending,
  waitForActiveSessionForegroundActivity,
} from "../../helpers/session-store";
import { seedIdleSession } from "../../helpers/session";
import { holdCancellationSettlement } from "../../helpers/cancellation-observation";

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
    const cancellation = await holdCancellationSettlement(testPage);
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Mobile background cancellation availability",
    );
    const sessionId = await session.activeChat().getAttribute("data-session-id");
    if (!sessionId) throw new Error("The active chat panel has no session ID");

    await session.sendMessageViaButton("/detached-background 60s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 20_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await waitForActiveSessionForegroundActivity(testPage, "background", sessionId);

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

    cancellation.arm(sessionId);
    try {
      await cancel.tap();
      await waitForActiveSessionCancellationPending(testPage, true, sessionId);
      await expect(cancel).toBeDisabled();
    } finally {
      cancellation.release();
    }
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await waitForActiveSessionCancellationPending(testPage, false, sessionId);
    await waitForActiveSessionForegroundActivity(testPage, null, sessionId, 75_000);
    await expect(cancel).not.toBeVisible({ timeout: 15_000 });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile background cancellation");
  });
});
