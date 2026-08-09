import { describe, it, expect } from "vitest";
import { mergeParkedProjection } from "./session-parked-merge";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";

// Flagged in PR review (#2476) as untested: the task-level twin
// (task-parked-merge.ts) has tasks-parked.test.ts; this file had none.
// mergeParkedProjection is a pure function, so it's tested directly rather
// than through the WS-handler/store harness tasks-parked.test.ts uses.

const TS = "2026-04-20T00:00:00Z";

function makeSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    state: "WAITING_FOR_INPUT",
    started_at: TS,
    updated_at: TS,
    ...overrides,
  };
}

describe("mergeParkedProjection (AC-39, D1)", () => {
  it("preserves the existing triple untouched when the frame omits parked_revision", () => {
    const existing = makeSession({
      parked_on_background_work: true,
      parked_epoch: 100,
      parked_revision: 7,
    });
    const incoming = makeSession({ parked_revision: undefined });

    expect(mergeParkedProjection(existing, incoming)).toEqual({
      parked_on_background_work: true,
      parked_epoch: 100,
      parked_revision: 7,
    });
  });

  it("accepts a strictly higher epoch even carrying a lower revision (AC-77 restart reset)", () => {
    const existing = makeSession({
      parked_on_background_work: true,
      parked_epoch: 100,
      parked_revision: 7,
    });
    const incoming = makeSession({
      parked_on_background_work: false,
      parked_epoch: 200,
      parked_revision: 0,
    });

    expect(mergeParkedProjection(existing, incoming)).toEqual({
      parked_on_background_work: false,
      parked_epoch: 200,
      parked_revision: 0,
    });
  });

  it("discards a stale (same-epoch, lower) revision — existing fields survive unchanged", () => {
    const existing = makeSession({
      parked_on_background_work: true,
      parked_epoch: 100,
      parked_revision: 7,
    });
    const incoming = makeSession({
      parked_on_background_work: false,
      parked_epoch: 100,
      parked_revision: 6,
    });

    expect(mergeParkedProjection(existing, incoming)).toEqual({
      parked_on_background_work: true,
      parked_epoch: 100,
      parked_revision: 7,
    });
  });

  it("accepts a tie on equal revision within the same epoch (>= is inclusive)", () => {
    const existing = makeSession({
      parked_on_background_work: true,
      parked_epoch: 100,
      parked_revision: 7,
    });
    const incoming = makeSession({
      parked_on_background_work: false,
      parked_epoch: 100,
      parked_revision: 7,
    });

    expect(mergeParkedProjection(existing, incoming)).toEqual({
      parked_on_background_work: false,
      parked_epoch: 100,
      parked_revision: 7,
    });
  });

  it("pins the ?? fallback contract: an incoming frame that wins on (epoch, revision) but omits parked_on_background_work falls back to the existing bool", () => {
    // The backend always sends the three parked fields together (this is a
    // documented, currently-unreachable-in-production shape), but the
    // fallback exists in the source and its behavior is part of the
    // contract, so it is pinned here rather than left implicit.
    const existing = makeSession({
      parked_on_background_work: true,
      parked_epoch: 100,
      parked_revision: 7,
    });
    const incoming = makeSession({
      parked_on_background_work: undefined,
      parked_epoch: 100,
      parked_revision: 8,
    });

    expect(mergeParkedProjection(existing, incoming)).toEqual({
      parked_on_background_work: true,
      parked_epoch: 100,
      parked_revision: 8,
    });
  });
});
