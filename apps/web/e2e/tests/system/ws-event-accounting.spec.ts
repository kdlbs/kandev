/**
 * WS event accounting — Workstream 1 e2e coverage.
 *
 * Per-connection seq detection (Phase 1) catches "FE never saw event N for
 * this connection." It cannot catch cross-session misrouting — an event for
 * session A delivered to session B's handler with a valid per-connection seq.
 *
 * Workstream 1 adds a per-session monotonic seq stamped at write time
 * (`BroadcastToSession`, e.g. `session.message.added`). Per-session buckets in
 * `WsAccount.bySession` only populate from events that arrive LIVE on the
 * current WS connection carrying a `session_seq`. Messages loaded via REST on
 * page load do NOT populate the bucket. So to exercise the per-session path
 * deterministically we must:
 *
 *   1. Navigate to a session page FIRST (so the connection subscribes), then
 *   2. Drive a LIVE message (`sendMessage`) and wait for the agent reply to
 *      render — that fires a live `session.message.added` on the current
 *      connection, populating that session's bucket.
 *
 * Then we assert the bucket is gap-free and that the backend's per-session
 * ring buffer agrees (no dropped session-routed events for that session).
 */

import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { computeWsDrops, formatDroppedEvents, readWsAccount } from "../../helpers/ws-account";

test.describe("WS event accounting — per-session sequencing", () => {
  test("a live session message produces a gap-free per-session stream", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // Seed a single task + session.
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "ws-account live session",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Navigate to the session page FIRST so the WS connection subscribes to
    // this session before we drive a live event. (Events that arrived before
    // navigation are loaded via REST on page load and do NOT carry a live
    // session_seq into the per-session bucket.)
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardByTitle("ws-account live session");
    await expect(card).toBeVisible({ timeout: 30_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for the initial (auto-started) turn to finish so the input is idle
    // and ready to accept a new prompt.
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    // Drive a LIVE message on the current connection. This fires a
    // `session.message.added` (and the streamed agent reply) carrying a
    // session_seq, populating the per-session bucket. We use a DISTINCT reply
    // text (not the initial "/e2e:simple-message" response) so the wait below
    // matches a unique string rather than `nth(1)` — the latter races on which
    // of two identical responses renders first under worker contention.
    const liveReply = "ws-account live reply marker";
    await session.sendMessage(`e2e:message("${liveReply}")`);

    // Wait for the new reply to render.
    await expect(session.chat.getByText(liveReply, { exact: false })).toBeVisible({
      timeout: 30_000,
    });
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });

    // Pull the WS accounting snapshot. The live message must have populated at
    // least one per-session bucket.
    const snap = await readWsAccount(testPage);
    expect(snap, "WS account hook must be installed under KANDEV_E2E_MOCK").not.toBeNull();
    const bySession = snap?.bySession ?? {};
    const sessionIds = Object.keys(bySession);

    expect(sessionIds.length, "expected at least one session bucket").toBeGreaterThan(0);

    for (const sessionId of sessionIds) {
      const bucket = bySession[sessionId];
      expect(
        bucket.gaps,
        `session ${sessionId}: per-session_seq gaps detected (${bucket.gaps.join(",")})`,
      ).toEqual([]);
    }

    // Cross-check against the backend ring buffer per session — surfaces drops
    // even when the FE's per-session bucket is locally gap-free (a session's
    // first event was missed entirely; the FE has no anchor min seq to detect
    // the gap from the per-session ring alone).
    for (const sessionId of sessionIds) {
      const drops = await computeWsDrops(testPage, apiClient, { sessionId });
      expect(drops, formatDroppedEvents(drops)).toEqual([]);
    }
  });
});
