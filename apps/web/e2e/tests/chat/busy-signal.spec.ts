import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { typeWhileBusy } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";

// ---------------------------------------------------------------------------
// ADR-0035 — operator-visible surfacing.
//
// The mock `/background <dur>` command spawns a top-level subagent Task and then
// holds the turn open with NO foreground output, so the session sits in the
// background-idle substate: RUNNING (working affordance up) yet promptable.
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

  test("background-idle session shows working AND accepts input, then flips to done", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskAndWaitForIdle(testPage, apiClient, seedData, "Busy signal (b)");

    // Spawn background work: foreground yields to a held-open subagent.
    await session.sendMessage("/background 12s");

    // The turn is working…
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });

    // …and once it yields to background work the composer flips to accept-input
    // (idle placeholder) even though the status still reads "running": the two
    // independent facts — "you may type" AND "work is still in progress" — are
    // both visible at once. This is the whole point of the fine-grained signal.
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await expect(session.agentStatus()).toBeVisible();

    // The working affordance must NOT be the done state while background runs.
    await expect(session.turnComplete()).toHaveCount(0);

    // Once the background subagent completes, the turn ends → done/idle and the
    // working affordance clears.
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

    await session.sendMessage("/background 20s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });

    // Wait for the background-idle window (accept-input while still working).
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await expect(session.agentStatus()).toBeVisible();

    // Type and submit — the message is accepted and posted, not diverted.
    const editor = testPage.locator(".tiptap.ProseMirror").first();
    await editor.click();
    await editor.fill("are you still working?");
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await editor.press(`${modifier}+Enter`);

    // It appears in the conversation as a sent user message…
    await expect(testPage.getByText("are you still working?")).toBeVisible({ timeout: 15_000 });
    // …and was NOT silently diverted to the queue.
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible();
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
    await session.sendMessage("/background 20s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });

    // Reach the background-idle window: accept-input while still working.
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await expect(session.agentStatus()).toBeVisible();

    // Reload: this is a fresh client that loads MID background-window. The
    // substate is not persisted and no activity_changed WS flip is due (the turn
    // was already background before the reload), so the only way the composer can
    // show accept-input + working here is if the boot payload carried the
    // fine-grained substate. This is the exact gap this batch closes: before it,
    // a reload showed the coarse "Queue more instructions…" busy affordance until
    // the next flip — which, for a genuinely idle-on-background turn, never comes.
    await testPage.reload();
    await session.waitForLoad();

    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
    await expect(session.agentStatus()).toBeVisible();
    await expect(session.turnComplete()).toHaveCount(0);
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
