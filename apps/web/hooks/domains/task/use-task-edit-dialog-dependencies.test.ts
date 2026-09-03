import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import {
  getTaskDependencies,
  replaceTaskDependencies,
} from "@/lib/api/domains/task-dependencies-api";
import { listTasksByWorkspace } from "@/lib/api/domains/kanban-api";
import {
  isTaskDependencyUpdateFailure,
  useTaskEditDialogDependencies,
} from "./use-task-edit-dialog-dependencies";

vi.mock("@/lib/api/domains/task-dependencies-api", () => ({
  getTaskDependencies: vi.fn(),
  replaceTaskDependencies: vi.fn(),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  listTasksByWorkspace: vi.fn(),
}));

const dependency = (id: string, title: string) => ({ id, title, state: "TODO" as const });

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  vi.mocked(getTaskDependencies).mockReset();
  vi.mocked(replaceTaskDependencies).mockReset();
  vi.mocked(listTasksByWorkspace).mockReset();
  vi.mocked(getTaskDependencies).mockResolvedValue({
    id: "task-1",
    depends_on: [dependency("task-2", "Predecessor")],
  });
  vi.mocked(listTasksByWorkspace).mockResolvedValue({
    total: 3,
    tasks: [
      { id: "task-1", title: "Edited task", archived_at: null },
      { id: "task-2", title: "Predecessor", archived_at: null },
      { id: "task-archived", title: "Archived", archived_at: "2026-01-01" },
    ],
  } as never);
  vi.mocked(replaceTaskDependencies).mockResolvedValue({ task_id: "task-1" });
});

describe("useTaskEditDialogDependencies", () => {
  it("loads current predecessors and filters the edited and archived tasks", async () => {
    const { result } = renderHook(() =>
      useTaskEditDialogDependencies({ open: true, workspaceId: "ws-1", taskId: "task-1" }),
    );

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.confirmedIds).toEqual(["task-2"]);
    expect(result.current.draftIds).toEqual(["task-2"]);
    expect(result.current.selectedTitles).toEqual({ "task-2": "Predecessor" });
    expect(result.current.candidates.map((task) => task.id)).toEqual(["task-2"]);
    expect(listTasksByWorkspace).toHaveBeenCalledWith("ws-1", {
      page: 1,
      pageSize: 100,
      query: "",
    });
  });

  it("loads every candidate page", async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => ({
      id: `task-${index + 1}`,
      title: `Task ${index + 1}`,
      archived_at: null,
    }));
    vi.mocked(listTasksByWorkspace).mockImplementation(async (_workspaceId, params) => {
      if (params?.page === 1) return { total: 101, tasks: firstPage } as never;
      return {
        total: 101,
        tasks: [{ id: "task-101", title: "Task 101", archived_at: null }],
      } as never;
    });

    const { result } = renderHook(() =>
      useTaskEditDialogDependencies({ open: true, workspaceId: "ws-1", taskId: "task-1" }),
    );

    await waitFor(() => expect(result.current.candidates).toHaveLength(100));

    expect(result.current.candidates.map((task) => task.id)).toContain("task-101");
    expect(listTasksByWorkspace).toHaveBeenNthCalledWith(2, "ws-1", {
      page: 2,
      pageSize: 100,
      query: "",
    });
  });

  it("saves only draft changes and updates the confirmed set after success", async () => {
    const { result } = renderHook(() =>
      useTaskEditDialogDependencies({ open: true, workspaceId: "ws-1", taskId: "task-1" }),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => result.current.setDraftIds([]));
    expect(replaceTaskDependencies).not.toHaveBeenCalled();
    expect(result.current.isDirty).toBe(true);

    await act(async () => result.current.save());

    expect(replaceTaskDependencies).toHaveBeenCalledWith("task-1", []);
    expect(result.current.confirmedIds).toEqual([]);
    expect(result.current.isDirty).toBe(false);
  });

  it("keeps the draft and exposes the original error when replacement fails", async () => {
    const apiError = new ApiError("would create a dependency cycle", 409, {
      error: "would create a dependency cycle",
      cycle: ["task-1", "task-2", "task-1"],
    });
    vi.mocked(replaceTaskDependencies).mockRejectedValueOnce(apiError);
    const { result } = renderHook(() =>
      useTaskEditDialogDependencies({ open: true, workspaceId: "ws-1", taskId: "task-1" }),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => result.current.setDraftIds(["task-2", "task-3"]));
    let failure: unknown;
    await act(async () => {
      try {
        await result.current.save();
      } catch (error: unknown) {
        failure = error;
      }
    });

    expect(isTaskDependencyUpdateFailure(failure)).toBe(true);
    expect(result.current.confirmedIds).toEqual(["task-2"]);
    expect(result.current.draftIds).toEqual(["task-2", "task-3"]);
    expect(result.current.error).toBe(apiError);
    expect(isTaskDependencyUpdateFailure(result.current.submitError)).toBe(true);
  });
});

describe("useTaskEditDialogDependencies failure handling", () => {
  it("keeps the editor ready when candidate search fails", async () => {
    vi.mocked(listTasksByWorkspace).mockRejectedValueOnce(new Error("candidate search failed"));
    const { result } = renderHook(() =>
      useTaskEditDialogDependencies({ open: true, workspaceId: "ws-1", taskId: "task-1" }),
    );

    await waitFor(() => expect(result.current.candidateError).not.toBeNull());

    expect(result.current.loading).toBe(false);
    expect(result.current.ready).toBe(true);
  });

  it("retries candidates without reloading the projection or losing the draft", async () => {
    vi.mocked(listTasksByWorkspace).mockRejectedValueOnce(new Error("candidate search failed"));
    const { result } = renderHook(() =>
      useTaskEditDialogDependencies({ open: true, workspaceId: "ws-1", taskId: "task-1" }),
    );

    await waitFor(() => expect(result.current.candidateError).not.toBeNull());
    act(() => result.current.setDraftIds(["task-3"]));
    vi.mocked(listTasksByWorkspace).mockResolvedValueOnce({
      total: 1,
      tasks: [{ id: "task-3", title: "Recovered candidate", archived_at: null }],
    } as never);

    act(() => result.current.retryCandidates());
    await waitFor(() =>
      expect(result.current.candidates).toEqual([
        { id: "task-3", title: "Recovered candidate", isArchived: false },
      ]),
    );

    expect(result.current.draftIds).toEqual(["task-3"]);
    expect(getTaskDependencies).toHaveBeenCalledOnce();
  });

  it("ignores a save result from the previous task after the dialog changes identity", async () => {
    const saveRequest = deferred<{ task_id: string }>();
    vi.mocked(replaceTaskDependencies).mockReturnValue(saveRequest.promise as never);
    const { result, rerender } = renderHook(
      ({ open, workspaceId, taskId }: { open: boolean; workspaceId: string; taskId: string }) =>
        useTaskEditDialogDependencies({ open, workspaceId, taskId }),
      {
        initialProps: { open: true, workspaceId: "ws-1", taskId: "task-1" },
      },
    );
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => result.current.setDraftIds(["task-3"]));
    let savePromise!: Promise<void>;
    act(() => {
      savePromise = result.current.save();
    });

    vi.mocked(getTaskDependencies).mockResolvedValueOnce({
      id: "task-2",
      depends_on: [dependency("task-4", "New predecessor")],
    });
    vi.mocked(listTasksByWorkspace).mockResolvedValueOnce({
      total: 1,
      tasks: [{ id: "task-4", title: "New predecessor", archived_at: null }],
    } as never);
    rerender({ open: true, workspaceId: "ws-2", taskId: "task-2" });
    await waitFor(() => expect(result.current.draftIds).toEqual(["task-4"]));

    await act(async () => {
      saveRequest.resolve({ task_id: "task-1" });
      await savePromise;
    });

    expect(result.current.confirmedIds).toEqual(["task-4"]);
    expect(result.current.draftIds).toEqual(["task-4"]);
    expect(result.current.saveError).toBeNull();
  });
});
