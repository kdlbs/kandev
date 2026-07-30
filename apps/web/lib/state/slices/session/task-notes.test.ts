import { describe, it, expect } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionSlice } from "./session-slice";
import type { SessionSlice } from "./types";
import type { TaskNote } from "@/lib/types/http";

function makeStore() {
  return create<SessionSlice>()(immer(createSessionSlice));
}

const TASK_ID = "task-1";

function makeNote(overrides: Partial<TaskNote> = {}): TaskNote {
  return {
    id: "note-1",
    task_id: TASK_ID,
    content: "Hello",
    updated_by: "user",
    created_at: "2026-07-30T00:00:00Z",
    updated_at: "2026-07-30T00:00:00Z",
    ...overrides,
  };
}

describe("task note slice", () => {
  it("stores a task note and clears loading", () => {
    const store = makeStore();
    store.getState().setTaskNoteLoading(TASK_ID, true);

    store.getState().setTaskNote(TASK_ID, makeNote());

    expect(store.getState().taskNotes.byTaskId[TASK_ID]?.content).toBe("Hello");
    expect(store.getState().taskNotes.loadingByTaskId[TASK_ID]).toBe(false);
  });

  it("tracks saving state per task", () => {
    const store = makeStore();

    store.getState().setTaskNoteSaving(TASK_ID, true);

    expect(store.getState().taskNotes.savingByTaskId[TASK_ID]).toBe(true);
  });

  it("clearTaskNote removes note state", () => {
    const store = makeStore();
    store.getState().setTaskNote(TASK_ID, makeNote());
    store.getState().setTaskNoteLoading(TASK_ID, true);
    store.getState().setTaskNoteSaving(TASK_ID, true);

    store.getState().clearTaskNote(TASK_ID);

    expect(store.getState().taskNotes.byTaskId[TASK_ID]).toBeUndefined();
    expect(store.getState().taskNotes.loadingByTaskId[TASK_ID]).toBeUndefined();
    expect(store.getState().taskNotes.savingByTaskId[TASK_ID]).toBeUndefined();
  });
});
