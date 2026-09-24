import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import {
  activeTaskSessionId,
  waitForActiveSessionCancellationPending,
  waitForActiveSessionCancellationPendingOrSettled,
  waitForActiveSessionForegroundActivity,
  waitForActiveSessionSupportsSteering,
} from "../../helpers/session-store";
import { seedIdleSession } from "../../helpers/session";
import { seedRunningGeneratingSession } from "../../helpers/generating-session";

test.describe.serial("Cancel turn availability", () => {
  test.describe.configure({ retries: 1 });

  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF: "true",
      KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING: "true",
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("keeps the direct input cancel control available during a steerable turn", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { session } = await seedRunningGeneratingSession(
      testPage,
      apiClient,
      seedData,
      "Direct input cancellation availability",
    );

    await waitForActiveSessionSupportsSteering(testPage);
    await expect(session.activeChat().getByTestId("cancel-agent-button")).toBeVisible();
    await expect(session.activeChat().getByTestId("submit-message-button")).toBeVisible();

    const cancelButton = session.activeChat().getByTestId("cancel-agent-button");
    await cancelButton.click();
    await waitForActiveSessionCancellationPendingOrSettled(testPage);
    await expect
      .poll(async () => {
        if (!(await cancelButton.isVisible().catch(() => false))) return true;
        return cancelButton.isDisabled();
      })
      .toBe(true);
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
    await waitForActiveSessionCancellationPending(testPage, false);
    await waitForActiveSessionForegroundActivity(testPage, null);
    await expect(session.activeChat().getByTestId("cancel-agent-button")).not.toBeVisible({
      timeout: 15_000,
    });
  });

  test("keeps the cancel control available while detached background work runs", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const gateway = watchWs(testPage);
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Background input cancellation availability",
    );
    const sessionId = await activeTaskSessionId(testPage);

    await session.sendMessage("/detached-background 20s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await waitForActiveSessionForegroundActivity(testPage, "background", sessionId);
    await expect(session.activeChat().getByTestId("cancel-agent-button")).toBeVisible();
    await expect(session.activeChat().getByTestId("submit-message-button")).toBeVisible();

    const cancelButton = session.activeChat().getByTestId("cancel-agent-button");
    const cancellationPending = gateway.waitForEvent("session.cancellation_changed", {
      where: (payload) => payload.session_id === sessionId && payload.cancellation_pending === true,
    });
    const cancellationSettled = gateway.waitForEvent("session.cancellation_changed", {
      where: (payload) =>
        payload.session_id === sessionId && payload.cancellation_pending === false,
    });
    await cancelButton.click();
    await cancellationPending;
    await waitForActiveSessionCancellationPending(testPage, true, sessionId);
    await expect(cancelButton).toBeDisabled();
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
    await cancellationSettled;
    await waitForActiveSessionCancellationPending(testPage, false, sessionId);
    await waitForActiveSessionForegroundActivity(testPage, null, sessionId);
    await expect(session.activeChat().getByTestId("cancel-agent-button")).not.toBeVisible({
      timeout: 15_000,
    });
  });
});
