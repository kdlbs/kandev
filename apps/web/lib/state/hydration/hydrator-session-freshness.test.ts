import { describe, expect, it } from "vitest";
import { produce } from "immer";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { hydrateState } from "./hydrator";

const EARLIER = "2026-10-26T01:11:12.552000000Z";
const LATER = "2026-10-26T01:11:14.070603526Z";

function session(state: TaskSession["state"], updatedAt = LATER): TaskSession {
  return { id: "session", task_id: "task", state, updated_at: updatedAt } as TaskSession;
}

function hydrate(live: TaskSession, incoming: TaskSession) {
  const state = structuredClone(defaultState) as AppState;
  state.taskSessions.items[live.id] = live;
  state.taskSessionsByTask.itemsByTaskId[live.task_id] = [live];
  return produce(state, (draft) => {
    hydrateState(draft, {
      taskSessions: { items: { [incoming.id]: incoming } },
      taskSessionsByTask: { itemsByTaskId: { [incoming.task_id]: [incoming] } },
    } as Partial<AppState>);
  });
}

// @covers AC-UI-SESSION-REFRESH-EFFICIENCY-001.2
describe("session hydration freshness", () => {
  it("keeps a settled live session when optional enrichment returns an old starting row", () => {
    const live = session("WAITING_FOR_INPUT");
    const result = hydrate(live, session("STARTING", EARLIER));
    expect(result.taskSessions.items.session).toMatchObject(live);
    expect(result.taskSessionsByTask.itemsByTaskId.task[0]).toMatchObject(live);
  });

  it("does not seed a settled turn boundary from a rejected old session", () => {
    const result = hydrate(session("RUNNING"), session("WAITING_FOR_INPUT", EARLIER));
    expect(result.taskSessions.items.session.state).toBe("RUNNING");
    expect(result.taskSessionsByTask.itemsByTaskId.task[0].state).toBe("RUNNING");
    expect(result.turns.settledBoundaryBySession.session).toBeUndefined();
  });

  it("keeps the canonical state for stale by-task-only hydration", () => {
    const live = session("WAITING_FOR_INPUT");
    const state = structuredClone(defaultState) as AppState;
    state.taskSessions.items[live.id] = live;
    state.taskSessionsByTask.itemsByTaskId[live.task_id] = [live];
    const result = produce(state, (draft) => {
      hydrateState(draft, {
        taskSessionsByTask: {
          itemsByTaskId: { task: [session("STARTING", EARLIER)] },
          loadingByTaskId: {},
          loadedByTaskId: {},
        },
      } as Partial<AppState>);
    });
    expect(result.taskSessionsByTask.itemsByTaskId.task[0]).toMatchObject(live);
  });

  it("preserves sub-millisecond ordering", () => {
    const live = session("RUNNING", "2026-10-26T01:11:14.000000002Z");
    const result = hydrate(live, session("STARTING", "2026-10-26T01:11:14.000000001Z"));
    expect(result.taskSessions.items.session).toMatchObject(live);
  });

  it("accepts a newer legitimate restart", () => {
    const incoming = { ...session("STARTING"), agent_execution_id: "replacement" };
    const result = hydrate(session("WAITING_FOR_INPUT", EARLIER), incoming);
    expect(result.taskSessions.items.session).toMatchObject(incoming);
    expect(result.taskSessionsByTask.itemsByTaskId.task[0]).toMatchObject(incoming);
  });

  it("retains equal-timestamp and timestamp-free hydration behavior", () => {
    expect(
      hydrate(session("STARTING"), session("WAITING_FOR_INPUT")).taskSessions.items.session.state,
    ).toBe("WAITING_FOR_INPUT");
    expect(
      hydrate(session("STARTING", ""), session("WAITING_FOR_INPUT", "")).taskSessions.items.session
        .state,
    ).toBe("WAITING_FOR_INPUT");
  });
});
