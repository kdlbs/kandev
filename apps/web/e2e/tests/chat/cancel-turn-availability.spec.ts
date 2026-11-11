import path from "node:path";
import { randomUUID } from "node:crypto";
import { test, expect } from "../../fixtures/test-base";
import { holdCancellationSettlement } from "../../helpers/cancellation-observation";
import {
  activeTaskSessionId,
  waitForActiveSessionCancellationPending,
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
    backend,
  }) => {
    test.setTimeout(120_000);
    const cancellation = await holdCancellationSettlement(testPage);
    const releaseMarker = path.join(backend.tmpDir, `cancel-availability-${randomUUID()}`);
    const { session, sessionId } = await seedRunningGeneratingSession(
      testPage,
      apiClient,
      seedData,
      "Direct input cancellation availability",
      {
        predecessorPrompt: `e2e:message("Steerable cancellation ready")\ne2e:await_file(${JSON.stringify(releaseMarker)})`,
      },
    );
    await expect(
      session.activeChat().getByText("Steerable cancellation ready", { exact: true }),
    ).toBeVisible();

    await waitForActiveSessionSupportsSteering(testPage);
    const activeSessionChat = testPage
      .locator(`[data-testid="session-chat"][data-session-id="${sessionId}"]:visible`)
      .first();
    await expect(activeSessionChat.getByTestId("cancel-agent-button")).toBeVisible();
    await expect(activeSessionChat.getByTestId("submit-message-button")).toBeVisible();

    const cancelButton = activeSessionChat.getByTestId("cancel-agent-button");
    cancellation.arm(sessionId);
    try {
      await cancelButton.click();
      await expect
        .poll(cancellation.pendingObserved, {
          message: "the WebSocket barrier should receive the pending cancellation event",
          timeout: 15_000,
        })
        .toBe(true);
      await waitForActiveSessionCancellationPending(testPage, true);
      await expect(cancelButton).toBeDisabled();
    } finally {
      cancellation.release();
    }
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
    await waitForActiveSessionCancellationPending(testPage, false);
    await waitForActiveSessionForegroundActivity(testPage, null);
    await expect(activeSessionChat.getByTestId("cancel-agent-button")).not.toBeVisible({
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
