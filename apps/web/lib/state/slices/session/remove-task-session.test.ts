import { describe, it, expect, beforeEach } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import { createSessionRuntimeSlice } from "../session-runtime/session-runtime-slice";
import type { SessionSlice } from "./types";
import type { SessionRuntimeSlice } from "../session-runtime/types";
import type { Message } from "@/lib/types/http";

type CombinedSlice = SessionSlice & SessionRuntimeSlice;

/** Creates a zustand store combining the session and session-runtime slices. */
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

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const INCARNATION_ID = `incarnation-1`;

describe("removeTaskSession cleanup cascade", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("purges messages, turns, and runtime buffers for the removed session", () => {
    const s = store.getState();
    s.registerSessionEnvironment(SESSION_ID, "env-1");
    s.appendShellOutput(SESSION_ID, "shell output");
    s.setContextWindow(SESSION_ID, {
      size: 1,
      used: 1,
      remaining: 0,
      efficiency: 1,
      compactionCount: 0,
    });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    s.addMessage({ id: "m1", session_id: SESSION_ID, role: "user", content: "hi" } as any);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    s.addTurn({ id: "t1", session_id: SESSION_ID } as any);
    s.markTurnsLoaded(SESSION_ID);
    // Seed the other two per-session reconciliation maps so removal must
    // clear all three: a leaked settled boundary would block every future
    // turn of a recreated session with the same id.
    s.reconcileWorkspaceSourcesAdopted([SESSION_ID], "2026-07-23T10:00:00Z");

    expect(store.getState().messages.bySession[SESSION_ID]).toHaveLength(1);
    s.replacePromptMessages(SESSION_ID, [
      {
        id: "prompt-1",
        session_id: SESSION_ID,
        task_id: TASK_ID,
        author_type: "user",
        type: "message",
        content: "prompt",
        created_at: "2026-08-22T00:00:00Z",
      } as unknown as Message,
    ]);
    expect(store.getState().messagePrompts.bySession[SESSION_ID]).toHaveLength(1);
    expect(store.getState().turns.bySession[SESSION_ID]).toHaveLength(1);
    expect(store.getState().turns.loadedBySession[SESSION_ID]).toBe(true);
    expect(store.getState().turns.settledBoundaryBySession[SESSION_ID]).toBe(
      "2026-07-23T10:00:00Z",
    );
    expect(store.getState().turns.reconcileEpochBySession[SESSION_ID]).toBe(1);

    store.getState().removeTaskSession(TASK_ID, SESSION_ID);

    const after = store.getState();
    expect(after.messages.bySession[SESSION_ID]).toBeUndefined();
    expect(after.messagePrompts.bySession[SESSION_ID]).toBeUndefined();
    expect(after.messagePrompts.generationBySession[SESSION_ID]).toBe(1);
    expect(after.turns.bySession[SESSION_ID]).toBeUndefined();
    expect(after.turns.loadedBySession[SESSION_ID]).toBeUndefined();
    expect(after.turns.settledBoundaryBySession[SESSION_ID]).toBeUndefined();
    expect(after.turns.reconcileEpochBySession[SESSION_ID]).toBeUndefined();
    expect(after.contextWindow.bySessionId[SESSION_ID]).toBeUndefined();
    expect(after.shell.outputs["env-1"]).toBeUndefined();
    expect(after.environmentIdBySessionId[SESSION_ID]).toBeUndefined();
  });

  it("clears queue metadata and protects a replacement operation token", () => {
    store.setState((draft) => {
      draft.taskSessions.items[SESSION_ID] = {
        id: SESSION_ID,
        task_id: TASK_ID,
        queue_incarnation_id: INCARNATION_ID,
      } as never;
    });
    expect(store.getState().beginQueueOperation(SESSION_ID, "stale-incarnation")).toBeNull();
    const first = store.getState().beginQueueOperation(SESSION_ID, INCARNATION_ID);
    expect(first).not.toBeNull();
    expect(store.getState().beginQueueOperation(SESSION_ID, INCARNATION_ID)).toBeNull();
    store.getState().setQueueEntries(SESSION_ID, [], {
      count: 0,
      max: 10,
      autoRun: true,
      mergeEnabled: true,
      sessionIncarnationId: INCARNATION_ID,
    });

    store.getState().removeTaskSession(TASK_ID, SESSION_ID);
    expect(store.getState().queue.metaBySessionId[SESSION_ID]).toBeUndefined();
    expect(store.getState().queue.activeOperationBySessionId[SESSION_ID]).toBeUndefined();

    store.setState((draft) => {
      draft.taskSessions.items[SESSION_ID] = {
        id: SESSION_ID,
        task_id: TASK_ID,
        queue_incarnation_id: "incarnation-2",
      } as never;
    });
    const replacement = store.getState().beginQueueOperation(SESSION_ID, "incarnation-2");
    expect(replacement).not.toBeNull();
    store.getState().finishQueueOperation(SESSION_ID, first!);
    expect(store.getState().queue.activeOperationBySessionId[SESSION_ID]).toEqual(replacement);
  });
});
