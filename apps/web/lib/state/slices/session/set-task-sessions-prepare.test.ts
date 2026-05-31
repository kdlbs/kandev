import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import { createSessionRuntimeSlice } from "../session-runtime/session-runtime-slice";
import type { SessionSlice } from "./types";
import type { SessionRuntimeSlice } from "../session-runtime/types";
import { sessionId as toSessionId, taskId as toTaskId, type TaskSession } from "@/lib/types/http";

type CombinedSlice = SessionSlice & SessionRuntimeSlice;

function makeStore() {
  return create<CombinedSlice>()(
    immer((set) => ({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionSlice as any)(set),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionRuntimeSlice as any)(set),
    })),
  );
}

const TASK_ID = toTaskId("task-1");
const TS = "2026-04-20T00:00:00Z";

function makeSession(id: string, metadata?: Record<string, unknown>): TaskSession {
  return {
    id: toSessionId(id),
    task_id: TASK_ID,
    state: "RUNNING",
    started_at: TS,
    updated_at: TS,
    ...(metadata ? { metadata } : {}),
  };
}

describe("setTaskSessionsForTask prepare backfill", () => {
  it("populates prepareProgress from session metadata.prepare_result", () => {
    const store = makeStore();
    const session = makeSession("s1", {
      prepare_result: {
        status: "completed",
        steps: [{ name: "clone", status: "ok", started_at: TS }],
      },
    });

    store.getState().setTaskSessionsForTask(TASK_ID, [session]);

    const prepare = store.getState().prepareProgress.bySessionId["s1"];
    expect(prepare).toBeDefined();
    expect(prepare.status).toBe("completed");
    expect(prepare.steps).toEqual([{ name: "clone", status: "ok", startedAt: TS }]);
  });

  it("does not create an entry for sessions without prepare_result", () => {
    const store = makeStore();
    store.getState().setTaskSessionsForTask(TASK_ID, [makeSession("s1")]);
    expect(store.getState().prepareProgress.bySessionId["s1"]).toBeUndefined();
  });

  it("does not clobber existing live prepare progress", () => {
    const store = makeStore();
    // Simulate live WS progress already in the store.
    store.setState((draft) => {
      draft.prepareProgress.bySessionId["s1"] = {
        sessionId: "s1",
        status: "running",
        steps: [{ name: "clone", status: "running" }],
      };
    });

    store.getState().setTaskSessionsForTask(TASK_ID, [
      makeSession("s1", {
        prepare_result: { status: "completed", steps: [] },
      }),
    ]);

    // Existing (live) entry is preserved, not overwritten by stale metadata.
    expect(store.getState().prepareProgress.bySessionId["s1"].status).toBe("running");
  });
});
