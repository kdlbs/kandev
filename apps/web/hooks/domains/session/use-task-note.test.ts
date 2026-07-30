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

const QUEUED_EDIT = "queued edit";

// Sets a draft and advances partway through the debounce window (< 1500ms),
// leaving the autosave still queued — shared by the flush-on-teardown tests.
async function queueUnsavedEdit(result: { current: { setDraftContent: (v: string) => void } }) {
  await act(async () => {
    result.current.setDraftContent(QUEUED_EDIT);
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(500);
  });
  expect(mockUpdateTaskNote).not.toHaveBeenCalled();
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

describe("useTaskNote autosave flush on teardown", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    seedState();
    mockGetTaskNote.mockResolvedValue(null);
    mockUpdateTaskNote.mockResolvedValue(makeNote({ content: "Saved" }));
    mockDeleteTaskNote.mockResolvedValue(undefined);
  });

  it("flushes a queued autosave when the task changes before the debounce fires", async () => {
    seedState(makeNote({ content: "Seed note" }));
    const { result, rerender } = renderHook(({ id }: { id: string }) => useTaskNote(id), {
      initialProps: { id: TASK_ID },
    });
    await queueUnsavedEdit(result);

    seedState(makeNote({ content: "Seed note", task_id: "task-2" }));
    await act(async () => {
      rerender({ id: "task-2" });
    });

    expect(mockUpdateTaskNote).toHaveBeenCalledWith(TASK_ID, QUEUED_EDIT, undefined);
  });

  it("flushes a queued autosave on unmount", async () => {
    seedState(makeNote({ content: "Seed note" }));
    const { result, unmount } = renderHook(() => useTaskNote(TASK_ID));
    await queueUnsavedEdit(result);

    await act(async () => {
      unmount();
    });

    expect(mockUpdateTaskNote).toHaveBeenCalledWith(TASK_ID, QUEUED_EDIT, undefined);
  });

  it("resets the draft when switching to a task whose saved content coincidentally matches", async () => {
    const SAME_CONTENT = "Same text";
    const OTHER_TASK_ID = "task-2";
    storeState = {
      ...storeState,
      taskNotes: {
        byTaskId: {
          [TASK_ID]: makeNote({ content: SAME_CONTENT }),
          [OTHER_TASK_ID]: makeNote({
            id: "note-2",
            task_id: OTHER_TASK_ID,
            content: SAME_CONTENT,
          }),
        },
        loadingByTaskId: {},
        savingByTaskId: {},
      },
    };
    const { result, rerender } = renderHook(({ id }: { id: string }) => useTaskNote(id), {
      initialProps: { id: TASK_ID },
    });
    await queueUnsavedEdit(result);

    await act(async () => {
      rerender({ id: OTHER_TASK_ID });
    });

    // Task A's leftover draft is flushed to task A, not silently carried
    // into task B just because their saved content strings matched.
    expect(mockUpdateTaskNote).toHaveBeenCalledWith(TASK_ID, QUEUED_EDIT, undefined);
    expect(mockUpdateTaskNote).not.toHaveBeenCalledWith(OTHER_TASK_ID, QUEUED_EDIT, undefined);
    expect(result.current.draftContent).toBe(SAME_CONTENT);
  });
});
