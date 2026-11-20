import type { PrAssetCapture } from "./pr-asset-capture";
import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";
import { watchWs, type WsWatcher } from "./causal-waits";
import {
  advanceCapacityRetries,
  driveCapacityRetries,
  freezeCapacityRetries,
  waitForCapacityRetry,
} from "./capacity-retry-clock";
import {
  createRetainedCapacityFixture,
  assertRetainedACPTrace,
  readMockACPTrace,
  waitForRetainedTurnFailure,
} from "./transient-turn-runtime-continuity";

async function driveDelayedCapacityRetries(
  ws: WsWatcher,
  backend: BackendContext,
  api: ApiClient,
  sessionId: string,
) {
  await driveCapacityRetries(backend, api, sessionId, 5, () =>
    ws.waitForEvent("session.message.updated", {
      where: (payload) => {
        const metadata = payload.metadata as Record<string, unknown> | undefined;
        return (
          payload.session_id === sessionId &&
          metadata?.recovery_phase === "waiting" &&
          metadata.attempt === 6
        );
      },
    }),
  );
}

export async function delayedCapacitySuccess(
  page: Page,
  backend: BackendContext,
  api: ApiClient,
  seed: SeedData,
  options: { mobile: boolean; capture: PrAssetCapture },
) {
  const { mobile, capture } = options;
  const ws = watchWs(page);
  const fixture = await createRetainedCapacityFixture(backend, api, seed, "completed-tools", {
    initialPrompt: "/capacity-completed-tools:6",
    controlRetryClock: true,
  });
  try {
    const session = new SessionPage(page);
    await page.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await expect(session.transientRetryCard()).toBeVisible();
    await expect(session.transientRetryCard()).toContainText("of 11");
    await expect(session.activeChat().getByTestId("retry-window-deadline")).toBeVisible();
    await driveDelayedCapacityRetries(ws, backend, api, fixture.sessionId);
    await waitForCapacityRetry(api, fixture.sessionId, 6);
    await expect(session.transientRetryCard()).toContainText("Attempt 6 of 11");
    await page.reload();
    await session.waitForLoad();
    await expect(session.transientRetryCard()).toHaveCount(1);
    const { messages: waitingMessages } = await api.listSessionMessages(fixture.sessionId);
    const waiting = waitingMessages.find(
      (row) => row.metadata?.retrying === true && row.metadata.attempt === 6,
    );
    expect(waiting).toBeDefined();
    // Match the browser's Date to the frozen backend clock for representative captures.
    await page.clock.setFixedTime(
      new Date(Date.parse(String(waiting!.metadata!.retry_at)) - 120_000),
    );
    await expect(session.transientRetryCard()).toContainText("2:00");
    const cancel = session.activeChat().getByTestId("recovery-cancel-retry-button");
    await expect(cancel).toBeVisible();
    if (mobile) {
      const bounds = await cancel.boundingBox();
      expect(bounds!.height).toBeGreaterThanOrEqual(44);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
    }
    await capture.screenshot(mobile ? "waiting-phone" : "waiting-desktop", {
      caption: "Capacity recovery after five attempts, preserving completed work.",
    });
    await driveOneFinalRetry(backend, api, fixture.sessionId);
    await expect(
      session
        .activeChat()
        .getByText(
          "Mock provider continued the unfinished request without repeating completed work.",
          { exact: true },
        ),
    ).toBeVisible();
    await expect(session.transientRetryCard()).toHaveCount(0);
    assertRetainedACPTrace(fixture.tracePath, 7);
    expect(
      readMockACPTrace(fixture.tracePath).filter((row) => row.event === "completed_side_effect"),
    ).toHaveLength(1);
  } finally {
    await fixture.dispose();
  }
}

async function driveOneFinalRetry(backend: BackendContext, api: ApiClient, sessionId: string) {
  const { messages } = await api.listSessionMessages(sessionId);
  const current = messages.find(
    (row) => row.metadata?.retrying === true && row.metadata.attempt === 6,
  );
  expect(current).toBeDefined();
  await advanceCapacityRetries(backend, Number(current!.metadata!.retry_in_seconds));
}

export async function delayedCapacityExpiry(
  page: Page,
  backend: BackendContext,
  api: ApiClient,
  seed: SeedData,
  capture: PrAssetCapture,
) {
  const fixture = await createRetainedCapacityFixture(backend, api, seed, "completed-tools", {
    initialPrompt: "/capacity-completed-tools:100",
    controlRetryClock: true,
  });
  try {
    const session = new SessionPage(page);
    await page.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await expect(session.transientRetryCard()).toBeVisible();
    await advanceCapacityRetries(backend, 1800);
    const failure = await waitForRetainedTurnFailure(api, fixture.sessionId, "window_expired");
    expect(failure.metadata?.attempts_started).toBe(0);
    await expect(session.activeChat().getByTestId("retained-turn-recovery-feedback")).toContainText(
      "recovery window ended",
    );
    await expect(session.transientRetryCard()).toHaveCount(0);
    await expect(session.activeChat().getByTestId("chat-input-area")).toBeVisible();
    await page.reload();
    await session.waitForLoad();
    await expect(session.activeChat().getByTestId("retained-turn-recovery-feedback")).toContainText(
      "recovery window ended",
    );
    await capture.screenshot("window-ended", {
      caption: "Recovery deadline reached without replaying completed work.",
    });
    assertRetainedACPTrace(fixture.tracePath, 1);
  } finally {
    await fixture.dispose();
  }
}

export async function delayedCapacityCancellation(
  page: Page,
  backend: BackendContext,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const ws = watchWs(page);
  const fixture = await createRetainedCapacityFixture(backend, api, seed, "completed-tools", {
    initialPrompt: "/capacity-completed-tools:100",
    controlRetryClock: true,
  });
  try {
    const session = new SessionPage(page);
    await page.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await driveDelayedCapacityRetries(ws, backend, api, fixture.sessionId);
    await waitForCapacityRetry(api, fixture.sessionId, 6);
    await expect(session.transientRetryCard()).toContainText("Attempt 6 of 11");
    const cancel = session.activeChat().getByTestId("recovery-cancel-retry-button");
    if (mobile) await cancel.tap();
    else await cancel.click();
    await expect(session.transientRetryCard()).toHaveCount(0);
    await advanceCapacityRetries(backend, 1800);
    if (mobile) await session.sendMessageViaButton("/e2e:simple-message");
    else await session.sendMessage("/e2e:simple-message");
    await expect(
      session
        .activeChat()
        .getByText("This is a simple mock response for e2e testing.", { exact: true }),
    ).toBeVisible();
    assertRetainedACPTrace(fixture.tracePath, 7);
  } finally {
    await fixture.dispose();
  }
}

export async function delayedCapacityRestart(
  page: Page,
  backend: BackendContext,
  api: ApiClient,
  seed: SeedData,
) {
  const ws = watchWs(page);
  const fixture = await createRetainedCapacityFixture(backend, api, seed, "completed-tools", {
    initialPrompt: "/capacity-completed-tools:100",
    controlRetryClock: true,
  });
  try {
    const session = new SessionPage(page);
    await page.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await driveDelayedCapacityRetries(ws, backend, api, fixture.sessionId);
    await waitForCapacityRetry(api, fixture.sessionId, 6);
    await expect(session.transientRetryCard()).toContainText("Attempt 6 of 11");
    await backend.restart({ KANDEV_DEBUG_PPROF_ENABLED: "true" });
    await freezeCapacityRetries(backend);
    await advanceCapacityRetries(backend, 1800);
    await page.reload();
    await session.waitForLoad();
    await expect(session.transientRetryCard()).toHaveCount(0);
    const { messages } = await api.listSessionMessages(fixture.sessionId);
    expect(
      messages.some((row) => row.metadata?.recovery_disposition === "restart_interrupted"),
    ).toBe(true);
    expect(
      readMockACPTrace(fixture.tracePath).filter((row) => row.event === "prompt"),
    ).toHaveLength(6);
    await expect(
      session
        .activeChat()
        .getByText("Updated fixture.txt before the provider reported capacity.", { exact: false }),
    ).toBeVisible();
  } finally {
    await fixture.dispose();
  }
}
