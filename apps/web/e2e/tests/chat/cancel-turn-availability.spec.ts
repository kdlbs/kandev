import { test, expect } from "../../fixtures/test-base";
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
  test.describe.configure({ retries: 0 });

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

    await session.activeChat().getByTestId("cancel-agent-button").click();
    // Detached background work has no foreground cancellation acknowledgement
    // to keep this transient progress flag observable in every browser frame.
    // The assertions below verify the user-visible activity settles instead.
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
    await waitForActiveSessionForegroundActivity(testPage, null, sessionId);
    await expect(session.activeChat().getByTestId("cancel-agent-button")).not.toBeVisible({
      timeout: 15_000,
    });
  });
});
