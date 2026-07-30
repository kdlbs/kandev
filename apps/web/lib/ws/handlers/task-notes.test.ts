import { describe, it, expect, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerTaskNotesHandlers } from "./task-notes";

const TASK_ID = "task-1";

function makeStore() {
  const state = {
    setTaskNote: vi.fn(),
    clearTaskNote: vi.fn(),
  };
  return {
    getState: () => state as unknown as AppState,
    setState: vi.fn(),
    subscribe: vi.fn(),
    destroy: vi.fn(),
    getInitialState: vi.fn(),
  } as unknown as StoreApi<AppState>;
}

function makeMessage(action: string, payload: Record<string, unknown>) {
  return { id: "msg-1", type: "notification", action, payload };
}

describe("task.note.* handlers", () => {
  it("stores updated notes", () => {
    const store = makeStore();
    const handlers = registerTaskNotesHandlers(store);

    handlers["task.note.updated"]!(
      makeMessage("task.note.updated", {
        id: "note-1",
        task_id: TASK_ID,
        content: "Hello",
        updated_by: "agent",
        created_at: "2026-07-30T00:00:00Z",
        updated_at: "2026-07-30T00:00:00Z",
      }) as never,
    );

    expect(store.getState().setTaskNote).toHaveBeenCalledWith(
      TASK_ID,
      expect.objectContaining({ content: "Hello" }),
    );
  });

  it("clears deleted notes", () => {
    const store = makeStore();
    const handlers = registerTaskNotesHandlers(store);

    handlers["task.note.deleted"]!(makeMessage("task.note.deleted", { task_id: TASK_ID }) as never);

    expect(store.getState().clearTaskNote).toHaveBeenCalledWith(TASK_ID);
  });
});
