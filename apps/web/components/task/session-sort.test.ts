import { describe, expect, it } from "vitest";
import {
  buildAgentLabelsById,
  pickActiveSessionId,
  resolveAgentLabelFor,
  sortSessions,
} from "./session-sort";
import type { TaskSession, TaskSessionState } from "@/lib/types/http";

const EPOCH = "2025-01-01T00:00:00Z";

function makeSession(overrides: Partial<TaskSession>): TaskSession {
  return {
    id: "s1",
    task_id: "t1",
    environment_id: "e1",
    state: "CREATED" as TaskSessionState,
    started_at: EPOCH,
    updated_at: EPOCH,
    ...overrides,
  } as TaskSession;
}

describe("sortSessions", () => {
  it("orders running sessions before completed ones", () => {
    const sessions = [
      makeSession({ id: "done", state: "COMPLETED", started_at: "2025-01-05T00:00:00Z" }),
      makeSession({ id: "run", state: "RUNNING", started_at: "2025-01-01T00:00:00Z" }),
    ];
    expect(sortSessions(sessions).map((s) => s.id)).toEqual(["run", "done"]);
  });

  it("breaks ties by most recent started_at", () => {
    const sessions = [
      makeSession({ id: "old", state: "RUNNING", started_at: "2025-01-01T00:00:00Z" }),
      makeSession({ id: "new", state: "RUNNING", started_at: "2025-01-05T00:00:00Z" }),
    ];
    expect(sortSessions(sessions).map((s) => s.id)).toEqual(["new", "old"]);
  });
});

describe("resolveAgentLabelFor", () => {
  it("prefers the current store label when the profile still exists", () => {
    const session = makeSession({
      agent_profile_id: "p1",
      agent_profile_snapshot: { label: "Snapshot Agent" },
    });
    const labels = buildAgentLabelsById([{ id: "p1", label: "Store Agent" } as never]);
    expect(resolveAgentLabelFor(session, labels)).toBe("Store Agent");
  });

  it("falls back to snapshot label when the profile is no longer in the store", () => {
    const session = makeSession({
      agent_profile_id: "deleted",
      agent_profile_snapshot: { label: "Snapshot Agent" },
    });
    expect(resolveAgentLabelFor(session, {})).toBe("Snapshot Agent");
  });

  it("returns 'Unknown agent' when both are missing", () => {
    const session = makeSession({ agent_profile_id: "missing" });
    expect(resolveAgentLabelFor(session, {})).toBe("Unknown agent");
  });
});

describe("pickActiveSessionId", () => {
  it("returns null for an empty list", () => {
    expect(pickActiveSessionId([], "anything")).toBeNull();
  });

  it("honors a preferred session when it exists", () => {
    const sessions = [makeSession({ id: "a" }), makeSession({ id: "b", is_primary: true })];
    expect(pickActiveSessionId(sessions, "a")).toBe("a");
  });

  it("ignores the preferred session when not present and falls back to primary", () => {
    const sessions = [makeSession({ id: "a" }), makeSession({ id: "b", is_primary: true })];
    expect(pickActiveSessionId(sessions, "missing")).toBe("b");
  });

  it("falls back to the first session when no primary exists", () => {
    const sessions = [makeSession({ id: "a" }), makeSession({ id: "b" })];
    expect(pickActiveSessionId(sessions, null)).toBe("a");
  });
});
