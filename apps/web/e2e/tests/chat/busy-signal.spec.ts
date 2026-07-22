import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { typeWhileBusy } from "../../helpers/type-while-busy";
import { waitForActiveSessionForegroundActivity } from "../../helpers/session-store";
import { SessionPage } from "../../pages/session-page";

// ---------------------------------------------------------------------------
// ADR-0049 — operator-visible surfacing.
//
// The mock `/detached-background <dur>` command returns its foreground response
// immediately while the launched workload continues, so this suite exercises
// work that genuinely outlives the foreground turn.
//
// The distinguishing observable of the background-idle window (b) is that the
// agent status still reads "running" (the working affordance) WHILE the composer
// shows its idle/accept placeholder — condition (a), a genuinely generating
// turn, keeps the "Queue more instructions…" busy placeholder and diverts input
// to the queue.
// ---------------------------------------------------------------------------

async function seedTaskAndWaitForIdle(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  return session;
}

test.describe("Fine-grained busy signal — composer + status", () => {
  test.describe.configure({ retries: 1 });

  test("async subagent accepts a new foreground turn and clears on singleton ID-less completion", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Async subagent lifecycle",
    );

    // This replay is deliberately ordered like Claude's async Agent path:
    // Agent launch (with agentId) -> human-origin foreground idle -> final
    // same-prompt thought/message -> prompt completion -> Claude's ID-less
    // task-notification completion. It is not the shell-background ordering
    // covered below. Exact-ID completion is pinned by backend unit coverage;
    // this browser path exercises the safe singleton fallback Claude requires.
    await session.sendMessage("/async-subagent-lifecycle 20s");
    await expect(testPage.getByText("Foreground response after async launch.")).toBeVisible({
      timeout: 20_000,
    });
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await waitForActiveSessionForegroundActivity(testPage, "background");
    const backgroundIndicator = session
      .sidebarTaskItem("Async subagent lifecycle")
      .getByTestId("task-state-background-running");
    await expect(backgroundIndicator).toBeVisible();

    // A prompt submitted while only the async child remains must be admitted
    // immediately. Foreground activity temporarily wins over the child.
    await session.sendMessage("/slow 2s");
    await expect(testPage.getByText("/slow 2s")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "generating");

    // The older async child remains visible after the second foreground yields,
    // then its singleton ID-less task-notification removes the final registration.
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
    await waitForActiveSessionForegroundActivity(testPage, "background");
    await waitForActiveSessionForegroundActivity(testPage, null);
    await expect(backgroundIndicator).not.toBeVisible();
  });

  test("execution teardown clears an async child whose completion never arrives", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Async subagent teardown",
      seedData.agentProfileId,
      {
        description: "/async-subagent-teardown",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("auto-started task did not return a session id");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await expect(session.agentStatus()).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "background");

    // No child completion is emitted by this scenario. The terminal execution
    // boundary must reconcile its owned registration instead.
    await apiClient.stopSession({
      session_id: task.session_id,
      reason: "e2e teardown",
      force: true,
    });
    await expect(session.agentStatus()).not.toBeVisible({ timeout: 20_000 });
    await waitForActiveSessionForegroundActivity(testPage, null);
    await expect(session.agentStatus()).not.toBeVisible();
  });

  test("background-idle session shows working AND accepts input, then flips to done", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(testPage, apiClient, seedData, "Busy signal (b)");

    // Launch work whose foreground prompt closes immediately.
    await session.sendMessage("/detached-background 12s");

    // The turn is working…
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });

    // …and once it yields to background work the composer flips to accept-input
    // (idle placeholder) even though the status still reads "running": the two
    // independent facts — "you may type" AND "work is still in progress" — are
    // both visible at once. This is the whole point of the fine-grained signal.
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await expect(session.agentStatus()).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "background");

    // The working affordance must NOT be the done state while background runs.
    // Once the detached workload completes, the working affordance clears.
    await expect(session.agentStatus()).not.toBeVisible({ timeout: 40_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 10_000 });
  });

  test("input typed during the background-idle window is sent, not queued", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(testPage, apiClient, seedData, "Busy signal send");

    await session.sendMessage("/detached-background 20s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });

    // Wait for the background-idle window (accept-input while still working).
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await expect(session.agentStatus()).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "background");

    // Type and submit a foreground turn — it is accepted and posted, not
    // diverted, and foreground busy temporarily takes absolute precedence.
    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await editor.click();
    await editor.fill("/slow 3s");
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await editor.press(`${modifier}+Enter`);

    // It appears in the conversation as a sent user message…
    await expect(testPage.getByText("/slow 3s")).toBeVisible({ timeout: 15_000 });
    // …and was NOT silently diverted to the queue.
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible();

    // While that foreground is active, the instant-send placeholder disappears
    // even though the older detached workload remains registered.
    await expect(session.idleInput()).not.toBeVisible({ timeout: 2_000 });
    await expect(testPage.locator('[data-placeholder^="Queue"]')).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "generating");

    // Foreground completion reveals the still-running detached work again.
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
    await expect(session.agentStatus()).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "background");
  });

  test("background-idle substate survives a fresh page reload (boot payload, no WS flip)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Busy signal reload",
    );

    // Open a long background window so it is still held after the reload lands.
    await session.sendMessage("/detached-background 20s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });

    // Reach the background-idle window: accept-input while still working.
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await expect(session.agentStatus()).toBeVisible();

    // Reload: this is a fresh client that loads MID background-window. The
    // substate is not persisted and no activity_changed WS flip is due, so the
    // only way the composer can
    // show accept-input + working here is if the boot payload carried the
    // fine-grained substate. This is the exact gap this batch closes: before it,
    // a reload showed the coarse "Queue more instructions…" busy affordance until
    // the next flip — which, for a genuinely idle-on-background turn, never comes.
    await testPage.reload();
    await session.waitForLoad();

    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
    await expect(session.agentStatus()).toBeVisible();
  });

  test("a turn with no background work keeps the composer gated (queues input)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Busy signal gated",
    );

    // A plain slow turn generates in the foreground the whole time — no
    // recognized background work — so the composer stays gated exactly as today.
    await session.sendMessage("/slow 10s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });

    // The accept-input (idle) placeholder must NOT appear while generating.
    await expect(session.idleInput()).not.toBeVisible();

    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await typeWhileBusy(testPage, editor, "should queue");
    const submitBtn = testPage.getByTestId("submit-message-button");
    await expect(submitBtn).toBeVisible({ timeout: 5_000 });
    await submitBtn.click();

    // Diverted to the queue, not posted — the historical contract is unchanged.
    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });
  });
});
