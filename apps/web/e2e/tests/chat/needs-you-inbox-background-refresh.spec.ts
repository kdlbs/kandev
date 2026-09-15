// AC-UI-NEEDS-YOU-INBOX-001: a workspace-wide session event must not flash
// the first-load "Loading…" state over an already-settled (here: empty)
// Inbox. Regression coverage for the `appliedGeneration`-gated view mode in
// needs-you-inbox-page-client.tsx.
import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";

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

    await testPage.goto("/needs-you-inbox");
    const emptyState = testPage.getByTestId("needs-you-inbox-empty");
    await expect(emptyState).toBeVisible({ timeout: 30_000 });
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

    resolveReleaseAll?.();
    // Still empty once the background reads actually resolve: the new task
    // never produced a clarification, so nothing should have changed.
    await expect(emptyState).toBeVisible({ timeout: 15_000 });
  });
});
