// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  waitForActiveSessionCancellationPending,
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
    const gateway = watchWs(testPage);
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

    const sessionId = await testPage.evaluate(() => {
      const store = (
        window as Window & {
          __KANDEV_E2E_STORE__?: {
            getState: () => { tasks: { activeSessionId: string | null } };
          };
        }
      ).__KANDEV_E2E_STORE__;
      return store?.getState().tasks.activeSessionId;
    });
    if (!sessionId) throw new Error("The active task session is not available");
    const cancellationPending = gateway.waitForEvent("session.cancellation_changed", {
      where: (payload) => payload.session_id === sessionId && payload.cancellation_pending === true,
    });
    const cancellationSettled = gateway.waitForEvent("session.cancellation_changed", {
      where: (payload) =>
        payload.session_id === sessionId && payload.cancellation_pending === false,
    });

    await cancel.tap();
    await cancellationPending;
    await expect
      .poll(async () => {
        if (!(await cancel.isVisible().catch(() => false))) return true;
        return cancel.isDisabled();
      })
      .toBe(true);
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await cancellationSettled;
    await waitForActiveSessionCancellationPending(testPage, false);
    await waitForActiveSessionForegroundActivity(testPage, null);
    await expect(cancel).not.toBeVisible({ timeout: 15_000 });
    await assertNoDocumentHorizontalOverflow(testPage, "mobile background cancellation");
  });
});
