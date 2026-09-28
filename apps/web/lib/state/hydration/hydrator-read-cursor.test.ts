import { describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { hydrateState } from "./hydrator";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

const SESSION_ID = "session-1";
const TASK_ID = "task-1";

function makeAppDraft(): AppState {
  return structuredClone(defaultState) as AppState;
}

function session(lastReadMessageId: string): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    last_read_message_id: lastReadMessageId,
  } as TaskSession;
}

describe("hydrateState task-session read cursors", () => {
  it("preserves a read cursor updated after the route snapshot started", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      const liveSession = session("message-2");
      draft.taskSessions.items[SESSION_ID] = liveSession;
      draft.taskSessions.readCursorEpochBySession = { [SESSION_ID]: 2 };
      draft.taskSessionsByTask.itemsByTaskId[TASK_ID] = [liveSession];

      hydrateState(
        draft,
        {
          taskSessions: { items: { [SESSION_ID]: session("message-1") } },
          taskSessionsByTask: {
            itemsByTaskId: { [TASK_ID]: [session("message-1")] },
            loadingByTaskId: { [TASK_ID]: false },
            loadedByTaskId: { [TASK_ID]: true },
          },
        } as unknown as Partial<AppState>,
        {
          taskSessionHydrationEpochsAtRequestStart: {
            [SESSION_ID]: { activity: 0, readCursor: 1 },
          },
        },
      );
    });

    expect(result.taskSessions.items[SESSION_ID].last_read_message_id).toBe("message-2");
    expect(result.taskSessionsByTask.itemsByTaskId[TASK_ID]?.[0]?.last_read_message_id).toBe(
      "message-2",
    );
    expect(result.taskSessions.readCursorEpochBySession?.[SESSION_ID]).toBe(2);
  });

  it("preserves a live cursor when a hydration has no captured request epoch", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      const liveSession = session("message-2");
      draft.taskSessions.items[SESSION_ID] = liveSession;
      draft.taskSessions.readCursorEpochBySession = { [SESSION_ID]: 1 };
      draft.taskSessionsByTask.itemsByTaskId[TASK_ID] = [liveSession];

      hydrateState(draft, {
        taskSessions: { items: { [SESSION_ID]: session("message-1") } },
        taskSessionsByTask: {
          itemsByTaskId: { [TASK_ID]: [session("message-1")] },
          loadingByTaskId: { [TASK_ID]: false },
          loadedByTaskId: { [TASK_ID]: true },
        },
      } as unknown as Partial<AppState>);
    });

    expect(result.taskSessions.items[SESSION_ID].last_read_message_id).toBe("message-2");
    expect(result.taskSessionsByTask.itemsByTaskId[TASK_ID]?.[0]?.last_read_message_id).toBe(
      "message-2",
    );
    expect(result.taskSessions.readCursorEpochBySession?.[SESSION_ID]).toBe(1);
  });

  it("advances the read cursor generation when a fresh route snapshot is accepted", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      const liveSession = session("message-1");
      draft.taskSessions.items[SESSION_ID] = liveSession;
      draft.taskSessions.readCursorEpochBySession = { [SESSION_ID]: 2 };
      draft.taskSessionsByTask.itemsByTaskId[TASK_ID] = [liveSession];

      hydrateState(
        draft,
        {
          taskSessions: { items: { [SESSION_ID]: session("message-3") } },
          taskSessionsByTask: {
            itemsByTaskId: { [TASK_ID]: [session("message-3")] },
            loadingByTaskId: { [TASK_ID]: false },
            loadedByTaskId: { [TASK_ID]: true },
          },
        } as unknown as Partial<AppState>,
        {
          taskSessionHydrationEpochsAtRequestStart: {
            [SESSION_ID]: { activity: 0, readCursor: 2 },
          },
        },
      );
    });

    expect(result.taskSessions.items[SESSION_ID].last_read_message_id).toBe("message-3");
    expect(result.taskSessions.readCursorEpochBySession?.[SESSION_ID]).toBe(3);
  });
});
