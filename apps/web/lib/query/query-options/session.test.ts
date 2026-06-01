import { describe, expect, it } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type Turn,
} from "@/lib/types/http";
import {
  deriveActiveTurnId,
  isSyntheticMessage,
  mergeSyntheticMessages,
  sessionByIdQueryOptions,
} from "./session";
import { qk } from "@/lib/query/keys";

const TS = "2026-05-30T00:00:00Z";
const NOTICE_T1 = "empty-turn-t1";

function turn(id: string, completed: boolean, overrides: Partial<Turn> = {}): Turn {
  return {
    id,
    session_id: toSessionId("sess-1"),
    task_id: toTaskId("task-1"),
    started_at: TS,
    completed_at: completed ? "2026-05-30T00:01:00Z" : undefined,
    created_at: TS,
    updated_at: TS,
    ...overrides,
  };
}

describe("deriveActiveTurnId", () => {
  it("returns null for an empty list", () => {
    expect(deriveActiveTurnId([])).toBeNull();
  });

  it("returns null when every turn has completed", () => {
    expect(deriveActiveTurnId([turn("t1", true), turn("t2", true)])).toBeNull();
  });

  it("returns the id of the in-progress turn", () => {
    expect(deriveActiveTurnId([turn("t1", true), turn("t2", false)])).toBe("t2");
  });

  it("returns the most recently started in-progress turn (last wins)", () => {
    // Two un-completed turns (e.g. a stale crashed one + the live one): the
    // current turn is the last in the chronologically-ordered list.
    expect(deriveActiveTurnId([turn("stale", false), turn("t2", true), turn("live", false)])).toBe(
      "live",
    );
  });

  it("treats a null completed_at as in-progress", () => {
    expect(deriveActiveTurnId([turn("t1", false, { completed_at: undefined })])).toBe("t1");
  });
});

function msg(id: string, turnId: string | undefined, synthetic: boolean): Message {
  return {
    id,
    session_id: toSessionId("sess-1"),
    task_id: toTaskId("task-1"),
    turn_id: turnId,
    author_type: synthetic ? "agent" : "user",
    content: "x",
    type: synthetic ? "status" : "message",
    metadata: synthetic ? { empty_turn: true } : undefined,
    created_at: TS,
  };
}

describe("isSyntheticMessage", () => {
  it("is true for empty-turn notices", () => {
    expect(isSyntheticMessage(msg(NOTICE_T1, "t1", true))).toBe(true);
  });
  it("is false for regular messages", () => {
    expect(isSyntheticMessage(msg("m1", "t1", false))).toBe(false);
  });
});

describe("mergeSyntheticMessages", () => {
  it("carries a synthetic notice whose turn is still in the server window", () => {
    const server = [msg("u1", "t1", false)];
    const prev = [msg("u1", "t1", false), msg(NOTICE_T1, "t1", true)];
    const merged = mergeSyntheticMessages(server, prev);
    expect(merged.map((m) => m.id)).toEqual(["u1", NOTICE_T1]);
  });

  it("drops a synthetic notice whose turn scrolled out of the server window", () => {
    const server = [msg("u2", "t2", false)];
    const prev = [msg(NOTICE_T1, "t1", true)];
    expect(mergeSyntheticMessages(server, prev)).toEqual(server);
  });

  it("does not duplicate a notice the server already returned", () => {
    const server = [msg("u1", "t1", false), msg(NOTICE_T1, "t1", true)];
    const prev = [msg(NOTICE_T1, "t1", true)];
    const merged = mergeSyntheticMessages(server, prev);
    expect(merged.filter((m) => m.id === NOTICE_T1)).toHaveLength(1);
  });

  it("returns the server list untouched when there is no prior cache", () => {
    const server = [msg("u1", "t1", false)];
    expect(mergeSyntheticMessages(server, undefined)).toBe(server);
  });

  it("ignores non-synthetic prior messages (no resurrection of deleted messages)", () => {
    const server = [msg("u2", "t2", false)];
    const prev = [msg("u1", "t1", false)];
    expect(mergeSyntheticMessages(server, prev)).toEqual(server);
  });
});

describe("sessionByIdQueryOptions", () => {
  it("targets the by-id cache key", () => {
    const opts = sessionByIdQueryOptions("sess-1");
    expect(opts.queryKey).toEqual(qk.taskSession.byId("sess-1"));
  });

  it("is observe-only (enabled:false) so it never fetches — bridge/SSR populate it", () => {
    const opts = sessionByIdQueryOptions("sess-1");
    expect(opts.enabled).toBe(false);
  });
});
