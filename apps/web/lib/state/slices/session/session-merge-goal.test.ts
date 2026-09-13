import { describe, expect, it } from "vitest";
import { mergeTaskSession } from "./session-merge";
import type { TaskSession } from "@/lib/types/http";

const ACTIVE_GOAL = {
  objective: "Coordinate contributor PR reviews",
  status: "active",
  createdAt: 10,
  updatedAt: 20,
};
const LIVE_GOAL_SNAPSHOT_AT = "2026-06-11T00:02:00.000Z";
const STALE_GOAL_SNAPSHOT_AT = "2026-06-11T00:01:30.000Z";
const FRESH_GOAL_SNAPSHOT_AT = "2026-06-11T00:03:00.000Z";

function session(
  metadata: Record<string, unknown>,
  extras: Partial<TaskSession> = {},
): TaskSession {
  return {
    id: "session-1",
    task_id: "task-1",
    state: "WAITING_FOR_INPUT",
    started_at: "2026-06-11T00:00:00.000Z",
    updated_at: "2026-06-11T00:01:00.000Z",
    metadata,
    ...extras,
  } as TaskSession;
}

describe("TaskSession goal hydration reconciliation", () => {
  it("does not resurrect a goal cleared by a live event", () => {
    const existing = session(
      { acp: { session_id: "acp-1", meta: { goal: null } } },
      {
        goal_reconciliation: {
          revision: 2,
          cleared: true,
          watermark: { createdAt: 10, updatedAt: 20 },
        },
      },
    );
    const staleHydration = session({
      acp: { session_id: "acp-1", meta: { goal: ACTIVE_GOAL } },
    });

    const merged = mergeTaskSession(existing, staleHydration);

    expect(merged.metadata?.acp).toMatchObject({ meta: { goal: null } });
    expect(merged.goal_reconciliation).toEqual(existing.goal_reconciliation);
  });

  it("does not let stale hydration clear a newer live goal", () => {
    const existing = session(
      {
        acp: {
          session_id: "acp-1",
          updated_at: LIVE_GOAL_SNAPSHOT_AT,
          meta: { goal: ACTIVE_GOAL },
        },
      },
      {
        goal_reconciliation: {
          revision: 1,
          cleared: false,
          watermark: { createdAt: 10, updatedAt: 20 },
          sourceUpdatedAt: LIVE_GOAL_SNAPSHOT_AT,
        },
      },
    );

    const merged = mergeTaskSession(
      existing,
      session({
        acp: {
          session_id: "acp-1",
          updated_at: STALE_GOAL_SNAPSHOT_AT,
          meta: { goal: null },
        },
      }),
    );

    expect(merged.metadata?.acp).toMatchObject({ meta: { goal: ACTIVE_GOAL } });
  });

  it("accepts a newer goal after a clear", () => {
    const existing = session(
      { acp: { session_id: "acp-1", meta: { goal: null } } },
      {
        goal_reconciliation: {
          revision: 2,
          cleared: true,
          watermark: { createdAt: 10, updatedAt: 20 },
        },
      },
    );
    const newerGoal = { ...ACTIVE_GOAL, createdAt: 11, updatedAt: 1 };

    const merged = mergeTaskSession(
      existing,
      session({
        acp: { session_id: "acp-1", meta: { goal: newerGoal } },
      }),
    );

    expect(merged.metadata?.acp).toMatchObject({ meta: { goal: newerGoal } });
  });

  it("invalidates the old goal when the ACP attachment changes", () => {
    const existing = session(
      { acp: { session_id: "acp-1", meta: { goal: ACTIVE_GOAL } } },
      {
        goal_reconciliation: {
          revision: 1,
          cleared: false,
          watermark: { createdAt: 10, updatedAt: 20 },
        },
      },
    );

    const attached = mergeTaskSession(
      existing,
      session({ acp: { session_id: "acp-2", meta: {} } }),
    );

    expect(attached.metadata?.acp).toMatchObject({ session_id: "acp-2", meta: { goal: null } });
    expect(attached.goal_reconciliation).toMatchObject({
      cleared: true,
      watermark: null,
    });

    const staleHydration = mergeTaskSession(
      attached,
      session({ acp: { session_id: "acp-2", meta: { goal: ACTIVE_GOAL } } }),
    );
    expect(staleHydration.metadata?.acp).toMatchObject({ meta: { goal: null } });
  });
});

describe("TaskSession reconnect goal hydration", () => {
  it("accepts a fresh reconnect snapshot that clears a live goal", () => {
    const existing = session(
      {
        acp: {
          session_id: "acp-1",
          updated_at: LIVE_GOAL_SNAPSHOT_AT,
          meta: { goal: ACTIVE_GOAL },
        },
      },
      {
        goal_reconciliation: {
          revision: 1,
          cleared: false,
          watermark: { createdAt: 10, updatedAt: 20 },
          sourceUpdatedAt: LIVE_GOAL_SNAPSHOT_AT,
        },
      },
    );

    const merged = mergeTaskSession(
      existing,
      session({
        acp: {
          session_id: "acp-1",
          updated_at: FRESH_GOAL_SNAPSHOT_AT,
          meta: { goal: null },
        },
      }),
    );

    expect(merged.metadata?.acp).toMatchObject({ meta: { goal: null } });
    expect(merged.goal_reconciliation).toMatchObject({
      revision: 2,
      cleared: true,
      sourceUpdatedAt: FRESH_GOAL_SNAPSHOT_AT,
    });
  });
});
