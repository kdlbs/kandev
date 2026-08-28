import { afterEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import {
  __resetOfficeTaskContentSyncForTests,
  beginWrite,
  closeFieldEditor,
  endWrite,
  getCanonicalValue,
  isFieldGuarded,
  nextTaskSequence,
  openFieldEditor,
  recordRefetchCandidate,
  recordWriteSuccess,
  seedInitialCanonical,
} from "@/lib/state/office-task-content-sync";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import type { Task } from "./types";

const mockGetTask = vi.fn();
const mockListActivityForTarget = vi.fn();
const mockListComments = vi.fn();
const mockListTaskSessions = vi.fn();

vi.mock("@/lib/api/domains/office-api", () => ({
  getTask: (...args: unknown[]) => mockGetTask(...args),
  listActivityForTarget: (...args: unknown[]) => mockListActivityForTarget(...args),
  listComments: (...args: unknown[]) => mockListComments(...args),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  listTaskSessions: (...args: unknown[]) => mockListTaskSessions(...args),
}));

import { useIssueData, useTaskOptimisticHelpers } from "./page";

afterEach(() => {
  vi.clearAllMocks();
  __resetOfficeTaskContentSyncForTests();
});

const TS = "2026-05-01T00:00:00Z";
const TASK_ID = "t-1";
const TITLE_FIELD = "title";
const EDITED_TITLE = "Edited While Loading";
const ORIGINAL_TITLE = "Original Title";
const ORIGINAL_DESCRIPTION = "Original description";

const officeTaskFixture: OfficeTask = {
  id: TASK_ID,
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: ORIGINAL_TITLE,
  description: ORIGINAL_DESCRIPTION,
  status: "todo",
  priority: "medium",
  createdAt: TS,
  updatedAt: TS,
};

function makeStoreWrapper(initialTasks: OfficeTask[]) {
  function StoreSeed({ children }: { children: ReactNode }) {
    const api = useAppStoreApi();
    if (initialTasks.length > 0) {
      api.getState().setTasks(initialTasks);
    }
    return <>{children}</>;
  }
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider>
        <StoreSeed>{children}</StoreSeed>
      </StateProvider>
    );
  }
  return Wrapper;
}

function taskFixture(overrides: Partial<Task> = {}): Task {
  return {
    id: TASK_ID,
    workspaceId: "ws-1",
    identifier: "TASK-1",
    title: "Live Title",
    status: "todo",
    priority: "medium",
    labels: [],
    blockedBy: [],
    blocking: [],
    children: [],
    reviewers: [],
    approvers: [],
    decisions: [],
    createdBy: "user",
    createdAt: TS,
    updatedAt: TS,
    ...overrides,
  };
}

function makeTaskRefSetter(initialTask: Task = taskFixture()) {
  const taskRef: { current: Task | null } = { current: initialTask };
  const setTask: React.Dispatch<React.SetStateAction<Task | null>> = (updater) => {
    taskRef.current =
      typeof updater === "function"
        ? (updater as (prev: Task | null) => Task | null)(taskRef.current)
        : updater;
  };
  return { taskRef, setTask };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe("useIssueData — initial load guard preservation (AC-27/28/38/39)", () => {
  it("does not overwrite title/description with a stale GET response that resolves after a mid-flight commit", async () => {
    mockListActivityForTarget.mockResolvedValue({ activity: [] });
    mockListComments.mockResolvedValue({ comments: [] });
    mockListTaskSessions.mockResolvedValue({ sessions: [] });
    const { promise: getTaskPromise, resolve: resolveGetTask } = deferred<{
      task: OfficeTask;
      timeline: [];
    }>();
    mockGetTask.mockReturnValue(getTaskPromise);

    const Wrapper = makeStoreWrapper([officeTaskFixture]);
    const { result } = renderHook(() => useIssueData(TASK_ID), { wrapper: Wrapper });

    // The from-store fast path seeds the displayed task synchronously.
    await waitFor(() => expect(result.current.task?.title).toBe(ORIGINAL_TITLE));

    // Simulate a title commit landing in full (useCommitTaskTitle's actual
    // sequence: guard the field, apply optimistically, then record the
    // write's own success) while the initial GET is still in flight.
    const writeSequence = nextTaskSequence(TASK_ID);
    act(() => {
      beginWrite(TASK_ID, TITLE_FIELD, writeSequence);
      result.current.applyTaskPatch({ title: EDITED_TITLE });
      recordWriteSuccess(TASK_ID, TITLE_FIELD, EDITED_TITLE, "2026-05-01T00:00:05Z", writeSequence);
      endWrite(TASK_ID, TITLE_FIELD, writeSequence);
    });
    expect(result.current.task?.title).toBe(EDITED_TITLE);

    // The GET resolves with the DTO as it looked before that commit landed
    // (an older title, a different status — the merge must only protect
    // title/description, not block the whole update).
    await act(async () => {
      resolveGetTask({
        task: { ...officeTaskFixture, title: "Server Title", status: "in_progress" },
        timeline: [],
      });
      await getTaskPromise;
      await Promise.resolve();
      await Promise.resolve();
    });

    await waitFor(() => expect(result.current.task?.status).toBe("in_progress"));
    expect(result.current.task?.title).toBe(EDITED_TITLE);
    expect(result.current.task?.description).toBe(ORIGINAL_DESCRIPTION);
  });
});

describe("useIssueData — canonical seeded synchronously from the store cache (COR-001)", () => {
  it("seeds canonical title/description as soon as the store-cache fast path renders, before the authoritative GET resolves", async () => {
    mockListActivityForTarget.mockResolvedValue({ activity: [] });
    mockListComments.mockResolvedValue({ comments: [] });
    mockListTaskSessions.mockResolvedValue({ sessions: [] });
    // The GET is left permanently pending for this test: the render gate
    // makes the page interactive from the store cache alone, and canonical
    // must already be seeded in that window, not only once this resolves.
    const { promise: getTaskPromise } = deferred<{ task: OfficeTask; timeline: [] }>();
    mockGetTask.mockReturnValue(getTaskPromise);

    const Wrapper = makeStoreWrapper([officeTaskFixture]);
    const { result } = renderHook(() => useIssueData(TASK_ID), { wrapper: Wrapper });

    await waitFor(() => expect(result.current.task?.title).toBe(ORIGINAL_TITLE));

    // AC-9/AC-64: a title commit that fails during this window restores the
    // field's recorded canonical value. Without a canonical value seeded
    // here, that restore silently no-ops (getCanonicalValue returns
    // undefined) and the failed edit is left standing.
    expect(getCanonicalValue(TASK_ID, "title")).toBe(ORIGINAL_TITLE);
    expect(getCanonicalValue(TASK_ID, "description")).toBe(ORIGINAL_DESCRIPTION);
  });
});

describe("useIssueData — store-cache seed does not clobber an existing canonical value (P1 review fix)", () => {
  it("does not let the store-cache fast path's seed win an equal-timestamp tiebreak against an already-recorded canonical value", async () => {
    mockListActivityForTarget.mockResolvedValue({ activity: [] });
    mockListComments.mockResolvedValue({ comments: [] });
    mockListTaskSessions.mockResolvedValue({ sessions: [] });
    // Left pending: the seed-vs-clobber window under test is between the
    // store-cache fast path rendering and this GET resolving.
    const { promise: getTaskPromise } = deferred<{ task: OfficeTask; timeline: [] }>();
    mockGetTask.mockReturnValue(getTaskPromise);

    // Simulates a canonical value already recorded on an earlier visit (e.g.
    // a write still pending when the user last navigated away), at the same
    // instant as the store-cached task seeded below.
    seedInitialCanonical(TASK_ID, "title", "Pending Write Title", TS);

    const Wrapper = makeStoreWrapper([officeTaskFixture]);
    const { result } = renderHook(() => useIssueData(TASK_ID), { wrapper: Wrapper });

    await waitFor(() => expect(result.current.task?.title).toBe(ORIGINAL_TITLE));

    // Without the if-absent guard, the store-cache fast path's own seed call
    // (issued at a higher sequence, same instant as the pre-existing
    // canonical) would win the AC-63 tiebreak and clobber it.
    expect(getCanonicalValue(TASK_ID, "title")).toBe("Pending Write Title");
  });
});

describe("useIssueData — initial load failure still seeds canonical (AC-59/AC-72)", () => {
  it("seeds canonical title/description from the store cache when the authoritative GET throws", async () => {
    mockListActivityForTarget.mockResolvedValue({ activity: [] });
    mockListComments.mockResolvedValue({ comments: [] });
    mockListTaskSessions.mockResolvedValue({ sessions: [] });
    mockGetTask.mockRejectedValue(new Error("network down"));

    const Wrapper = makeStoreWrapper([officeTaskFixture]);
    const { result } = renderHook(() => useIssueData(TASK_ID), { wrapper: Wrapper });

    // The from-store fast path renders the task interactively; the
    // authoritative GET then fails outright.
    await waitFor(() => expect(result.current.task?.title).toBe(ORIGINAL_TITLE));

    // AC-9/AC-64: a later failed title commit restores the recorded
    // canonical value. If the authoritative load never completes, the page
    // still renders (and lets the user edit) the store-cached title, so a
    // canonical value must exist for that restore to have a target.
    await waitFor(() => expect(getCanonicalValue(TASK_ID, "title")).toBe(ORIGINAL_TITLE));
    expect(getCanonicalValue(TASK_ID, "description")).toBe(ORIGINAL_DESCRIPTION);
  });
});

describe("useIssueData — guard release does not spuriously patch an unset description (AC-71)", () => {
  it("leaves a no-description task's description undefined after a no-op description-editor open/close", async () => {
    mockListActivityForTarget.mockResolvedValue({ activity: [] });
    mockListComments.mockResolvedValue({ comments: [] });
    mockListTaskSessions.mockResolvedValue({ sessions: [] });
    const taskNoDescription: OfficeTask = { ...officeTaskFixture, description: undefined };
    mockGetTask.mockResolvedValue({ task: taskNoDescription, timeline: [] });

    const Wrapper = makeStoreWrapper([taskNoDescription]);
    const { result } = renderHook(() => ({ issue: useIssueData(TASK_ID), api: useAppStoreApi() }), {
      wrapper: Wrapper,
    });

    await waitFor(() => expect(result.current.issue.task?.title).toBe(ORIGINAL_TITLE));
    await waitFor(() => expect(result.current.issue.loading).toBe(false));

    // No-op: a user opens the description editor to check whether there's a
    // description, then cancels without typing anything. The canonical value
    // is normalised to "" (AC-71), but the store/display never held a
    // description in the first place, so nothing should be written.
    act(() => {
      openFieldEditor(TASK_ID, "description");
      closeFieldEditor(TASK_ID, "description");
    });

    expect(result.current.issue.task?.description).toBeUndefined();
    expect(
      result.current.api.getState().office.tasks.items.find((t) => t.id === TASK_ID)?.description,
    ).toBeUndefined();
  });
});

describe("useTaskOptimisticHelpers — unmount cleanup order (AC-68)", () => {
  it("flushes a canonical value recorded while guarded to the store on unmount", () => {
    openFieldEditor(TASK_ID, TITLE_FIELD);
    const sequence = nextTaskSequence(TASK_ID);
    recordRefetchCandidate(TASK_ID, TITLE_FIELD, "Flushed On Release", TS, sequence);

    const Wrapper = makeStoreWrapper([officeTaskFixture]);
    // Mirrors the real invariant: by the time a listener could fire, the
    // page has already finished its initial load, so `task` is non-null.
    const { taskRef, setTask } = makeTaskRefSetter();
    const setTimeline = vi.fn();

    const { result, unmount } = renderHook(
      () => ({
        helpers: useTaskOptimisticHelpers(TASK_ID, setTask, setTimeline),
        api: useAppStoreApi(),
      }),
      { wrapper: Wrapper },
    );
    const storeApi = result.current.api;

    unmount();

    expect(storeApi.getState().office.tasks.items.find((t) => t.id === TASK_ID)?.title).toBe(
      "Flushed On Release",
    );
    expect(taskRef.current?.title).toBe("Flushed On Release");
  });

  it("reconciles the store once a write still pending at unmount time resolves (P1 review fix)", () => {
    const sequence = nextTaskSequence(TASK_ID);
    beginWrite(TASK_ID, TITLE_FIELD, sequence);

    const Wrapper = makeStoreWrapper([officeTaskFixture]);
    const { taskRef, setTask } = makeTaskRefSetter();
    const setTimeline = vi.fn();

    const { result, unmount } = renderHook(
      () => ({
        helpers: useTaskOptimisticHelpers(TASK_ID, setTask, setTimeline),
        api: useAppStoreApi(),
      }),
      { wrapper: Wrapper },
    );
    const storeApi = result.current.api;

    unmount();

    // The field is still guarded by the pending write, so the listener must
    // stay registered past unmount instead of unsubscribing unconditionally.
    expect(isFieldGuarded(TASK_ID, TITLE_FIELD)).toBe(true);

    act(() => {
      recordWriteSuccess(
        TASK_ID,
        TITLE_FIELD,
        "Reconciled After Unmount",
        "2026-05-01T00:00:05Z",
        sequence,
      );
      endWrite(TASK_ID, TITLE_FIELD, sequence);
    });

    expect(isFieldGuarded(TASK_ID, TITLE_FIELD)).toBe(false);
    expect(taskRef.current?.title).toBe("Reconciled After Unmount");
    expect(storeApi.getState().office.tasks.items.find((t) => t.id === TASK_ID)?.title).toBe(
      "Reconciled After Unmount",
    );
  });

  it("never restores title/description when rolling back an unrelated picker mutation (AC-61)", () => {
    const Wrapper = makeStoreWrapper([officeTaskFixture]);
    const { taskRef, setTask } = makeTaskRefSetter();
    const setTimeline = vi.fn();

    const { result } = renderHook(() => useTaskOptimisticHelpers(TASK_ID, setTask, setTimeline), {
      wrapper: Wrapper,
    });

    // A stale snapshot from before a since-confirmed title edit landed.
    const staleSnapshot: Task = { ...taskRef.current!, title: "Stale Title", priority: "high" };
    act(() => {
      result.current.restoreTask(staleSnapshot);
    });

    expect(taskRef.current?.title).toBe("Live Title");
    expect(taskRef.current?.priority).toBe("high");
  });
});

describe("useTaskOptimisticHelpers — task id changes with pending writes", () => {
  function renderTaskHelpers() {
    const oldTask = taskFixture({ id: "t-1", title: "Old title", description: "Old description" });
    const newTask = taskFixture({ id: "t-2", title: "New title", description: "New description" });
    const { taskRef, setTask } = makeTaskRefSetter(oldTask);
    const setTimeline = vi.fn();
    const Wrapper = makeStoreWrapper([]);
    const rendered = renderHook(
      ({ taskId }: { taskId: string }) => useTaskOptimisticHelpers(taskId, setTask, setTimeline),
      { initialProps: { taskId: "t-1" }, wrapper: Wrapper },
    );
    return { ...rendered, oldTask, newTask, taskRef };
  }

  it("does not apply an old task's deferred title to the replacement task", () => {
    const sequence = nextTaskSequence("t-1");
    beginWrite("t-1", TITLE_FIELD, sequence);
    const { rerender, unmount, newTask, taskRef } = renderTaskHelpers();

    taskRef.current = newTask;
    rerender({ taskId: "t-2" });

    act(() => {
      recordWriteSuccess(
        "t-1",
        TITLE_FIELD,
        "Old title from server",
        "2026-05-01T00:00:05Z",
        sequence,
      );
      endWrite("t-1", TITLE_FIELD, sequence);
    });

    expect(taskRef.current).toEqual(newTask);
    unmount();
  });

  it("does not apply an old task's captured patch or restore to the replacement task", () => {
    const { result, rerender, unmount, newTask, oldTask, taskRef } = renderTaskHelpers();
    const oldApplyPatch = result.current.applyTaskPatch;
    const oldRestoreTask = result.current.restoreTask;

    taskRef.current = newTask;
    rerender({ taskId: "t-2" });

    act(() => {
      oldApplyPatch({ title: "Old optimistic title" });
      oldRestoreTask({ ...oldTask, title: "Old restored title", priority: "high" });
    });

    expect(taskRef.current).toEqual(newTask);
    unmount();
  });
});

describe("useIssueData — initial load task identity", () => {
  it("does not preserve the previous task's guarded fields after a route change", async () => {
    mockListActivityForTarget.mockResolvedValue({ activity: [] });
    mockListComments.mockResolvedValue({ comments: [] });
    mockListTaskSessions.mockResolvedValue({ sessions: [] });
    const firstLoad = deferred<{ task: OfficeTask; timeline: [] }>();
    const secondLoad = deferred<{ task: OfficeTask; timeline: [] }>();
    mockGetTask.mockImplementation((taskId: string) =>
      taskId === "t-1" ? firstLoad.promise : secondLoad.promise,
    );

    const Wrapper = makeStoreWrapper([
      { ...officeTaskFixture, title: "First task", description: "First description" },
    ]);
    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string }) => useIssueData(taskId),
      { initialProps: { taskId: "t-1" }, wrapper: Wrapper },
    );

    act(() => {
      firstLoad.resolve({
        task: { ...officeTaskFixture, title: "First task", description: "First description" },
        timeline: [],
      });
    });
    await waitFor(() => expect(result.current.task?.id).toBe("t-1"));
    rerender({ taskId: "t-2" });
    openFieldEditor("t-2", TITLE_FIELD);
    await act(async () => {
      secondLoad.resolve({
        task: {
          ...officeTaskFixture,
          id: "t-2",
          identifier: "TASK-2",
          title: "Second task",
          description: "Second description",
        },
        timeline: [],
      });
      await secondLoad.promise;
    });
    await waitFor(() => expect(result.current.task?.id).toBe("t-2"));

    expect(result.current.task?.title).toBe("Second task");
    expect(result.current.task?.description).toBe("Second description");
    closeFieldEditor("t-2", TITLE_FIELD);
  });
});
