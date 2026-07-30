import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskNote } from "@/lib/types/http";

const mockGetTaskNote = vi.fn();
const mockUpdateTaskNote = vi.fn();
const mockDeleteTaskNote = vi.fn();
const mockSetTaskNote = vi.fn();
const mockSetTaskNoteLoading = vi.fn();
const mockSetTaskNoteSaving = vi.fn();
const mockClearTaskNote = vi.fn();

type MockStoreState = {
  taskNotes: {
    byTaskId: Record<string, TaskNote>;
    loadingByTaskId: Record<string, boolean>;
    savingByTaskId: Record<string, boolean>;
  };
  connection: { status: string };
  setTaskNote: typeof mockSetTaskNote;
  setTaskNoteLoading: typeof mockSetTaskNoteLoading;
  setTaskNoteSaving: typeof mockSetTaskNoteSaving;
  clearTaskNote: typeof mockClearTaskNote;
};

let storeState = {} as MockStoreState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockStoreState) => unknown) => selector(storeState),
}));

vi.mock("@/lib/api/domains/note-api", () => ({
  getTaskNote: (...args: unknown[]) => mockGetTaskNote(...args),
  updateTaskNote: (...args: unknown[]) => mockUpdateTaskNote(...args),
  deleteTaskNote: (...args: unknown[]) => mockDeleteTaskNote(...args),
}));

import { useTaskNote } from "./use-task-note";

const TASK_ID = "task-1";

function makeNote(overrides: Partial<TaskNote> = {}): TaskNote {
  return {
    id: "note-1",
    task_id: TASK_ID,
    content: "Seed note",
    updated_by: "user",
    created_at: "2026-07-30T00:00:00Z",
    updated_at: "2026-07-30T00:00:00Z",
    ...overrides,
  };
}

function seedState(note: TaskNote | null = null) {
  storeState = {
    taskNotes: {
      byTaskId: note === null ? {} : { [TASK_ID]: note },
      loadingByTaskId: {},
      savingByTaskId: {},
    },
    connection: { status: "connected" },
    setTaskNote: mockSetTaskNote,
    setTaskNoteLoading: mockSetTaskNoteLoading,
    setTaskNoteSaving: mockSetTaskNoteSaving,
    clearTaskNote: mockClearTaskNote,
  };
}

describe("useTaskNote", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    seedState();
    mockGetTaskNote.mockResolvedValue(null);
    mockUpdateTaskNote.mockResolvedValue(makeNote({ content: "Saved" }));
    mockDeleteTaskNote.mockResolvedValue(undefined);
  });

  it("fetches the note on mount", async () => {
    mockGetTaskNote.mockResolvedValue(makeNote());

    renderHook(() => useTaskNote(TASK_ID));

    await vi.runAllTimersAsync();

    expect(mockSetTaskNoteLoading).toHaveBeenCalledWith(TASK_ID, true);
    expect(mockGetTaskNote).toHaveBeenCalledWith(TASK_ID);
    expect(mockSetTaskNote).toHaveBeenCalledWith(
      TASK_ID,
      expect.objectContaining({ content: "Seed note" }),
    );
    expect(mockSetTaskNoteLoading).toHaveBeenLastCalledWith(TASK_ID, false);
  });

  it("debounces note autosaves", async () => {
    seedState(makeNote({ content: "Seed note" }));
    const { result } = renderHook(() => useTaskNote(TASK_ID));

    await act(async () => {
      result.current.setDraftContent("Updated note");
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1499);
    });
    expect(mockUpdateTaskNote).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });

    expect(mockUpdateTaskNote).toHaveBeenCalledWith(TASK_ID, "Updated note", undefined);
  });

  it("keeps a newer draft when a stale in-flight save echoes back", async () => {
    seedState(makeNote({ content: "Seed note" }));
    let resolveSave: (note: TaskNote) => void = () => {};
    mockUpdateTaskNote.mockImplementationOnce(
      () =>
        new Promise<TaskNote>((resolve) => {
          resolveSave = resolve;
        }),
    );

    const { result, rerender } = renderHook(() => useTaskNote(TASK_ID));

    await act(async () => {
      result.current.setDraftContent("v1");
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    expect(mockUpdateTaskNote).toHaveBeenCalledWith(TASK_ID, "v1", undefined);

    // User keeps typing while the "v1" save is still in flight.
    await act(async () => {
      result.current.setDraftContent("v2");
    });

    // The stale save resolves and lands in the store, echoing back "v1".
    await act(async () => {
      resolveSave(makeNote({ content: "v1" }));
      await Promise.resolve();
    });
    seedState(makeNote({ content: "v1" }));
    await act(async () => {
      rerender();
    });

    expect(result.current.draftContent).toBe("v2");

    // The newer draft still autosaves on its own cycle.
    mockUpdateTaskNote.mockResolvedValueOnce(makeNote({ content: "v2" }));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    expect(mockUpdateTaskNote).toHaveBeenLastCalledWith(TASK_ID, "v2", undefined);
  });

  it("deletes an existing note when the draft is cleared", async () => {
    seedState(makeNote({ content: "Seed note" }));
    const { result } = renderHook(() => useTaskNote(TASK_ID));

    await act(async () => {
      result.current.setDraftContent("");
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
      await Promise.resolve();
    });

    expect(mockDeleteTaskNote).toHaveBeenCalledWith(TASK_ID);
    expect(mockSetTaskNote).toHaveBeenCalledWith(TASK_ID, null);
  });
});
