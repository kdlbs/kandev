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
  const store = create<CombinedSlice>()(
    immer((set) => ({
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionSlice as any)(set),
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ...(createSessionRuntimeSlice as any)(set),
    })),
  );
  // Inject the workflow-step metadata that the session slice reads to order
  // sessions by step flow (the real app supplies this via the kanban slice).
  store.setState({
    kanban: { steps: [{ id: "spec", position: 0, title: "Spec" }] },
    kanbanMulti: {
      snapshots: {
        wf1: {
          steps: [
            { id: "work", position: 1, title: "Work" },
            { id: "review", position: 2, title: "Review" },
          ],
        },
      },
    },
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
  } as any);
  return store;
}

const TASK_ID = toTaskId("task-1");
const TS = "2026-04-20T00:00:00Z";

function makeSession(id: string, stepId: string | undefined, startedAt: string): TaskSession {
  return {
    id: toSessionId(id),
    task_id: TASK_ID,
    state: "RUNNING",
    started_at: startedAt,
    updated_at: startedAt,
    ...(stepId ? { workflow_step_id: stepId } : {}),
  } as TaskSession;
}

describe("session slice step-flow ordering", () => {
  it("stores sessions ordered by workflow-step position on setTaskSessionsForTask", () => {
    const store = makeStore();
    store
      .getState()
      .setTaskSessionsForTask(TASK_ID, [
        makeSession("review", "review", TS),
        makeSession("spec", "spec", "2026-04-20T00:00:05Z"),
        makeSession("work", "work", "2026-04-20T00:00:03Z"),
      ]);
    expect(store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID].map((s) => s.id)).toEqual([
      "spec",
      "work",
      "review",
    ]);
  });

  it("re-sorts when a session's step changes via an event upsert", () => {
    const store = makeStore();
    store
      .getState()
      .setTaskSessionsForTask(TASK_ID, [
        makeSession("a", "spec", TS),
        makeSession("b", "work", "2026-04-20T00:00:01Z"),
      ]);
    expect(store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID].map((s) => s.id)).toEqual([
      "a",
      "b",
    ]);

    // Session "a" moves forward to the review step — it should slot after "b".
    store.getState().upsertTaskSessionFromEvent(TASK_ID, makeSession("a", "review", TS));
    expect(store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID].map((s) => s.id)).toEqual([
      "b",
      "a",
    ]);
  });

  it("keeps sessions with no known step ordered last", () => {
    const store = makeStore();
    store
      .getState()
      .setTaskSessionsForTask(TASK_ID, [
        makeSession("quick", undefined, TS),
        makeSession("spec", "spec", "2026-04-20T00:00:05Z"),
      ]);
    expect(store.getState().taskSessionsByTask.itemsByTaskId[TASK_ID].map((s) => s.id)).toEqual([
      "spec",
      "quick",
    ]);
  });
});
