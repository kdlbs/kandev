import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

type MockTask = { id: string; primarySessionId?: string | null };
type MockStoreState = {
  tasks: { activeTaskId: string | null; activeSessionId: string | null };
  kanban: { tasks: MockTask[] };
};

let storeState = {} as MockStoreState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockStoreState) => unknown) => selector(storeState),
}));

import { resolveNoteSessionId, useNoteSessionId } from "./use-note-session-id";

const SESSION_ACTIVE = "session-active";
const SESSION_PRIMARY = "session-primary";
const TASK_1 = "task-1";
const TASK_2 = "task-2";

describe("resolveNoteSessionId", () => {
  it("follows the active session when the panel's task is the active task", () => {
    expect(
      resolveNoteSessionId({
        taskId: TASK_1,
        activeTaskId: TASK_1,
        activeSessionId: SESSION_ACTIVE,
        taskPrimarySessionId: SESSION_PRIMARY,
      }),
    ).toBe(SESSION_ACTIVE);
  });

  it("falls back to the task's own primary session when it isn't the active task", () => {
    expect(
      resolveNoteSessionId({
        taskId: TASK_2,
        activeTaskId: TASK_1,
        activeSessionId: SESSION_ACTIVE,
        taskPrimarySessionId: SESSION_PRIMARY,
      }),
    ).toBe(SESSION_PRIMARY);
  });

  it("returns null when taskId is null", () => {
    expect(
      resolveNoteSessionId({
        taskId: null,
        activeTaskId: null,
        activeSessionId: SESSION_ACTIVE,
        taskPrimarySessionId: null,
      }),
    ).toBeNull();
  });
});

describe("useNoteSessionId", () => {
  it("does not leak the active session onto a non-active task's enhance target", () => {
    storeState = {
      tasks: { activeTaskId: TASK_1, activeSessionId: SESSION_ACTIVE },
      kanban: { tasks: [{ id: TASK_2, primarySessionId: SESSION_PRIMARY }] },
    };

    const { result } = renderHook(() => useNoteSessionId(TASK_2));

    expect(result.current).toBe(SESSION_PRIMARY);
    expect(result.current).not.toBe(SESSION_ACTIVE);
  });

  it("uses the live active session when the panel shows the active task", () => {
    storeState = {
      tasks: { activeTaskId: TASK_1, activeSessionId: SESSION_ACTIVE },
      kanban: { tasks: [{ id: TASK_1, primarySessionId: SESSION_PRIMARY }] },
    };

    const { result } = renderHook(() => useNoteSessionId(TASK_1));

    expect(result.current).toBe(SESSION_ACTIVE);
  });
});
