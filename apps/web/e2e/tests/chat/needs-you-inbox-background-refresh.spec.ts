// AC-UI-NEEDS-YOU-INBOX-001: a workspace-wide session event must not flash
// the first-load "Loading…" state over an already-settled (here: empty)
// Inbox. Regression coverage for the `lastAppliedOk`-gated view mode in
// needs-you-inbox-page-client.tsx.
import { test, expect } from "../../fixtures/test-base";
import { watchWs, waitForHttp } from "../../helpers/causal-waits";

type InboxStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => { bumpNeedsYouInboxRefreshTick: () => void };
  };
};

async function triggerInboxRefresh(testPage: import("@playwright/test").Page) {
  await testPage.evaluate(() => {
    const store = (window as InboxStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge is unavailable");
    store.getState().bumpNeedsYouInboxRefreshTick();
  });
}

test.describe("Needs-you Inbox background refresh", () => {
  test("keeps the settled empty state during a WS-triggered background refresh", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // Arm before the first navigation: `page.on("websocket")` only sees
    // sockets opened after this call (see causal-waits.ts).
    const wsWatcher = watchWs(testPage);

    const initialInboxLoaded = waitForHttp(testPage, "GET", /\/api\/v1\/clarification-inbox$/);
    await testPage.goto("/needs-you-inbox");
    await initialInboxLoaded;
    const emptyState = testPage.getByTestId("needs-you-inbox-empty");
    await expect(emptyState).toBeVisible();
    const loadingText = testPage.getByText("Loading…", { exact: true });
    await expect(loadingText).toHaveCount(0);

    // Hold every clarification-inbox read that starts from here on,
    // deterministically, instead of racing a guessed wall-clock delay
    // against the real refresh. `/e2e:simple-message` fires several
    // workspace-wide session.state_changed / pending_action_changed events in
    // quick succession (CREATED, STARTING, ...); without holding every one of
    // them, a later un-held read resolves fast enough to re-settle the view
    // before the assertion below ever samples the DOM, hiding the exact race
    // this test exists to catch.
    let resolveHeld: (() => void) | undefined;
    const held = new Promise<void>((resolve) => {
      resolveHeld = resolve;
    });
    let resolveReleaseAll: (() => void) | undefined;
    const releaseAll = new Promise<void>((resolve) => {
      resolveReleaseAll = resolve;
    });
    await testPage.route("**/api/v1/clarification-inbox?*", async (route) => {
      const response = await route.fetch();
      resolveHeld?.();
      await releaseAll;
      await route.fulfill({ response });
    });

    const stateChanged = wsWatcher.waitForEvent("session.state_changed");

    // Any session transition in the workspace broadcasts `session.state_changed`
    // workspace-wide (design-02#Control-flow) and bumps the Inbox's refresh
    // tick, regardless of whether the task ever produces a clarification.
    const title = "Needs-you Inbox Background Refresh";
    await apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    await stateChanged;
    await held;

    // At least one background read is held in flight and none has resolved:
    // the settled empty view must survive it, and the first-load spinner
    // must not reappear.
    await expect(emptyState).toBeVisible();
    await expect(loadingText).toHaveCount(0);

    const backgroundReadResolved = waitForHttp(testPage, "GET", /\/api\/v1\/clarification-inbox$/);
    resolveReleaseAll?.();
    await backgroundReadResolved;
    // Still empty once the background reads actually resolve: the new task
    // never produced a clarification, so nothing should have changed.
    await expect(emptyState).toBeVisible();
  });

  test("coalesces mixed triggers and waits for Retry-After before recovering from 503", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);
    const initialInboxLoaded = waitForHttp(testPage, "GET", /\/api\/v1\/clarification-inbox$/);
    await testPage.goto("/needs-you-inbox");
    await initialInboxLoaded;

    let requestCount = 0;
    let resolveFirstStarted!: () => void;
    const firstStarted = new Promise<void>((resolve) => {
      resolveFirstStarted = resolve;
    });
    let releaseFirst!: () => void;
    const firstBarrier = new Promise<void>((resolve) => {
      releaseFirst = resolve;
    });
    let resolveRetryStarted!: (at: number) => void;
    const retryStarted = new Promise<number>((resolve) => {
      resolveRetryStarted = resolve;
    });
    let releaseRetry!: () => void;
    const retryBarrier = new Promise<void>((resolve) => {
      releaseRetry = resolve;
    });
    let resolveRetryFinished!: () => void;
    const retryFinished = new Promise<void>((resolve) => {
      resolveRetryFinished = resolve;
    });

    await testPage.route("**/api/v1/clarification-inbox?*", async (route) => {
      requestCount += 1;
      if (requestCount === 1) {
        resolveFirstStarted();
        await firstBarrier;
        await route.fulfill({
          status: 503,
          headers: { "Content-Type": "application/json", "Retry-After": "2" },
          body: JSON.stringify({ error: "temporarily busy", error_code: "clarification_busy" }),
        });
        return;
      }

      resolveRetryStarted(Date.now());
      const response = await route.fetch();
      await retryBarrier;
      await route.fulfill({ response });
      resolveRetryFinished();
    });

    try {
      await triggerInboxRefresh(testPage);
      await firstStarted;

      // While the request is held, mix store, foreground, and browser return
      // events. They share the held read and leave one pending refresh.
      await triggerInboxRefresh(testPage);
      await triggerInboxRefresh(testPage);
      await testPage.evaluate(() => {
        window.dispatchEvent(new Event("focus"));
        document.dispatchEvent(new Event("visibilitychange"));
        window.dispatchEvent(new Event("pageshow"));
        window.dispatchEvent(new Event("online"));
      });
      expect(requestCount).toBe(1);

      const failureResponse = waitForHttp(testPage, "GET", /\/api\/v1\/clarification-inbox$/, {
        predicate: (response) => response.status() === 503,
      });
      releaseFirst();
      await failureResponse;
      const failureObservedAt = Date.now();

      // Continued refresh signals during cooldown must not start another read.
      await triggerInboxRefresh(testPage);
      await triggerInboxRefresh(testPage);
      await testPage.evaluate(() => window.dispatchEvent(new Event("focus")));

      const retryAt = await retryStarted;
      expect(requestCount).toBe(2);
      expect(retryAt - failureObservedAt).toBeGreaterThanOrEqual(1_900);

      releaseRetry();
      await retryFinished;
    } finally {
      releaseFirst();
      releaseRetry();
    }
  });
});
