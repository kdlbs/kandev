import { describe, it, expect } from "vitest";

import { getSessionInfoForTask } from "./session-info";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";

type SessionOverrides = Partial<Omit<TaskSession, "id" | "task_id">> & {
  id?: string;
  task_id?: string;
};

function session(overrides: SessionOverrides): TaskSession {
  const { id, task_id, ...rest } = overrides;
  return {
    id: toSessionId(id ?? "s1"),
    task_id: toTaskId(task_id ?? "t1"),
    state: overrides.state ?? "WAITING_FOR_INPUT",
    started_at: overrides.started_at ?? "",
    updated_at: overrides.updated_at ?? "",
    is_primary: overrides.is_primary ?? false,
    ...rest,
  } as TaskSession;
}

describe("getSessionInfoForTask", () => {
  it("returns undefined sessionState when the task has no sessions", () => {
    const info = getSessionInfoForTask("t1", {}, {});
    expect(info.sessionState).toBeUndefined();
  });

  it("returns the primary session's state when it is the only session", () => {
    const info = getSessionInfoForTask(
      "t1",
      { t1: [session({ id: "p", is_primary: true, state: "WAITING_FOR_INPUT" })] },
      {},
    );
    expect(info.sessionState).toBe("WAITING_FOR_INPUT");
  });

  // Regression: the sidebar derives its "running" badge from this state.
  // A secondary chat tab that the user opens while the primary is idle must
  // still surface as "in progress" — without this the task keeps showing
  // "Turn Finished" while the new agent is working.
  it("returns RUNNING when any non-primary session is running, even if primary is idle", () => {
    const info = getSessionInfoForTask(
      "t1",
      {
        t1: [
          session({ id: "p", is_primary: true, state: "WAITING_FOR_INPUT" }),
          session({ id: "s", state: "RUNNING" }),
        ],
      },
      {},
    );
    expect(info.sessionState).toBe("RUNNING");
  });

  it("prefers RUNNING over STARTING", () => {
    const info = getSessionInfoForTask(
      "t1",
      {
        t1: [
          session({ id: "p", is_primary: true, state: "STARTING" }),
          session({ id: "s", state: "RUNNING" }),
        ],
      },
      {},
    );
    expect(info.sessionState).toBe("RUNNING");
  });

  it("falls back to the primary's state when no session is more active", () => {
    const info = getSessionInfoForTask(
      "t1",
      {
        t1: [
          session({ id: "p", is_primary: true, state: "WAITING_FOR_INPUT" }),
          session({ id: "s", state: "COMPLETED" }),
        ],
      },
      {},
    );
    expect(info.sessionState).toBe("WAITING_FOR_INPUT");
  });
});
