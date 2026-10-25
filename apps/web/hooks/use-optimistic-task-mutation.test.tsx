import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import {
  TaskOptimisticContextProvider,
  useCommitTaskDescription,
  useCommitTaskTitle,
  useOptimisticTaskMutation,
} from "./use-optimistic-task-mutation";
import type { Task } from "@/app/office/tasks/[id]/types";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import type { Task as HttpTask } from "@/lib/types/http";
import {
  __resetOfficeTaskContentSyncForTests,
  getCanonicalValue,
  seedInitialCanonical,
  subscribeField,
} from "@/lib/state/office-task-content-sync";

vi.mock("sonner", () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  updateTask: vi.fn(),
}));

import { toast } from "sonner";
import { updateTask } from "@/lib/api/domains/kanban-api";

const mockUpdateTask = vi.mocked(updateTask);

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  __resetOfficeTaskContentSyncForTests();
});

const TS = "2026-05-01T00:00:00Z";

const baseTask: Task = {
  id: "t-1",
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: "First task",
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
};

const baseOfficeTask: OfficeTask = {
  id: "t-1",
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: "First task",
  status: "todo",
  priority: "medium",
  createdAt: TS,
  updatedAt: TS,
};

function makeHarness(initialTask: Task, initialOffice: OfficeTask | null) {
  const state = {
    task: initialTask,
    patches: [] as Partial<Task>[],
    restored: [] as Task[],
  };
  const ctxValue = {
    task: initialTask,
    applyPatch: (patch: Partial<Task>) => {
      state.patches.push(patch);
      state.task = { ...state.task, ...patch };
    },
    restore: (snapshot: Task) => {
      state.restored.push(snapshot);
      state.task = snapshot;
    },
  };
  function StoreSeed({ children }: { children: ReactNode }) {
    const api = useAppStoreApi();
    if (initialOffice) {
      api.getState().setTasks([initialOffice]);
    }
    return <>{children}</>;
  }
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider>
        <StoreSeed>
          <TaskOptimisticContextProvider value={ctxValue}>{children}</TaskOptimisticContextProvider>
        </StoreSeed>
      </StateProvider>
    );
  }
  return { Wrapper, state };
}

function HookProbe({
  onReady,
}: {
  onReady: (mutate: ReturnType<typeof useOptimisticTaskMutation>) => void;
}) {
  const mutate = useOptimisticTaskMutation();
  onReady(mutate);
  return null;
}

describe("useOptimisticTaskMutation", () => {
  it("applies the patch and keeps it on success", async () => {
    const { Wrapper, state } = makeHarness(baseTask, baseOfficeTask);
    let mutate: ReturnType<typeof useOptimisticTaskMutation> | null = null;
    render(
      <Wrapper>
        <HookProbe onReady={(m) => (mutate = m)} />
      </Wrapper>,
    );
    expect(mutate).not.toBeNull();
    const apiCall = vi.fn().mockResolvedValue({ ok: true });
    await act(async () => {
      await mutate!("t-1", { priority: "high" }, apiCall);
    });
    expect(apiCall).toHaveBeenCalledTimes(1);
    expect(state.patches).toEqual([{ priority: "high" }]);
    expect(state.restored).toHaveLength(0);
    expect(state.task.priority).toBe("high");
  });

  it("rolls back local state and toasts on api failure", async () => {
    const { Wrapper, state } = makeHarness(baseTask, baseOfficeTask);
    let mutate: ReturnType<typeof useOptimisticTaskMutation> | null = null;
    render(
      <Wrapper>
        <HookProbe onReady={(m) => (mutate = m)} />
      </Wrapper>,
    );
    const apiCall = vi.fn().mockRejectedValue(new Error("nope"));
    await act(async () => {
      await expect(mutate!("t-1", { priority: "high" }, apiCall)).rejects.toThrow("nope");
    });
    expect(state.patches).toEqual([{ priority: "high" }]);
    expect(state.restored).toHaveLength(1);
    expect(state.restored[0]).toEqual(baseTask);
    expect(toast.error).toHaveBeenCalledWith("nope");
  });

  it("uses a generic error message when the rejection isn't an Error", async () => {
    const { Wrapper } = makeHarness(baseTask, baseOfficeTask);
    let mutate: ReturnType<typeof useOptimisticTaskMutation> | null = null;
    render(
      <Wrapper>
        <HookProbe onReady={(m) => (mutate = m)} />
      </Wrapper>,
    );
    const apiCall = vi.fn().mockRejectedValue("boom");
    await act(async () => {
      await expect(mutate!("t-1", { priority: "high" }, apiCall)).rejects.toBe("boom");
    });
    expect(toast.error).toHaveBeenCalledWith("Update failed");
  });

  it("never restores title/description in the store, even when a confirmed edit lands mid-flight (AC-61)", async () => {
    const { Wrapper } = makeHarness(baseTask, baseOfficeTask);
    let mutate: ReturnType<typeof useOptimisticTaskMutation> | null = null;
    let storeApi: ReturnType<typeof useAppStoreApi> | null = null;
    function Probe() {
      const m = useOptimisticTaskMutation();
      const s = useAppStoreApi();
      mutate = m;
      storeApi = s;
      return null;
    }
    render(
      <Wrapper>
        <Probe />
      </Wrapper>,
    );
    const { promise, reject } = deferred<unknown>();
    const apiCall = vi.fn().mockReturnValue(promise);

    let pending!: Promise<void>;
    act(() => {
      pending = mutate!("t-1", { priority: "high" }, apiCall);
    });

    // A title commit succeeds server-side and lands in the store while this
    // unrelated priority mutation is still in flight (e.g. useCommitTaskTitle
    // resolving concurrently).
    act(() => {
      storeApi!.getState().patchTaskInStore("t-1", { title: "Confirmed New Title" });
    });

    await act(async () => {
      reject(new Error("nope"));
      await expect(pending).rejects.toThrow("nope");
    });

    // The failed priority mutation's rollback must not clobber the
    // meanwhile-confirmed title back to its pre-mutation snapshot value.
    expect(storeApi!.getState().office.tasks.items[0]?.title).toBe("Confirmed New Title");
    expect(storeApi!.getState().office.tasks.items[0]?.priority).toBe("medium");
  });

  it("works when the office store has no entry for the task", async () => {
    const { Wrapper, state } = makeHarness(baseTask, null);
    let mutate: ReturnType<typeof useOptimisticTaskMutation> | null = null;
    render(
      <Wrapper>
        <HookProbe onReady={(m) => (mutate = m)} />
      </Wrapper>,
    );
    const apiCall = vi.fn().mockResolvedValue({ ok: true });
    await act(async () => {
      await mutate!("t-1", { status: "done" }, apiCall);
    });
    expect(state.patches).toEqual([{ status: "done" }]);
    expect(state.task.status).toBe("done");
  });
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function taskResponse(overrides: Partial<HttpTask> & { title: string; description: string }) {
  return {
    id: "t-1",
    workspace_id: "ws-1",
    workflow_id: "wf-1",
    workflow_step_id: "step-1",
    position: 0,
    state: "TODO",
    priority: "medium",
    created_at: TS,
    updated_at: TS,
    ...overrides,
  } as unknown as HttpTask;
}

const NEW_TITLE = "New Title";
const ORIGINAL_TITLE = "Original Title";
const WRITE_FAILURE_MESSAGE = "boom";
const WRITE_RESPONSE_TS = "2026-05-02T00:00:00Z";
const NEW_DESCRIPTION = "New description";

function TitleHookProbe({
  onReady,
}: {
  onReady: (
    commit: ReturnType<typeof useCommitTaskTitle>,
    storeApi: ReturnType<typeof useAppStoreApi>,
  ) => void;
}) {
  const commit = useCommitTaskTitle();
  const storeApi = useAppStoreApi();
  onReady(commit, storeApi);
  return null;
}

describe("useCommitTaskTitle", () => {
  it("patches the local task and store optimistically at issue time, and keeps it on success (AC-3, AC-11)", async () => {
    const { Wrapper, state } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskTitle> | null = null;
    let storeApi: ReturnType<typeof useAppStoreApi> | null = null;
    const { promise, resolve } = deferred<ReturnType<typeof taskResponse>>();
    mockUpdateTask.mockReturnValueOnce(promise);
    render(
      <Wrapper>
        <TitleHookProbe onReady={(c, s) => ([commit, storeApi] = [c, s])} />
      </Wrapper>,
    );

    let pending!: Promise<void>;
    act(() => {
      pending = commit!("t-1", NEW_TITLE);
    });
    expect(state.patches).toEqual([{ title: NEW_TITLE }]);
    expect(mockUpdateTask).toHaveBeenCalledWith("t-1", { title: NEW_TITLE });
    expect(storeApi!.getState().office.tasks.items[0]?.title).toBe(NEW_TITLE);

    await act(async () => {
      resolve(taskResponse({ title: NEW_TITLE, description: "", updated_at: WRITE_RESPONSE_TS }));
      await pending;
    });
    expect(state.task.title).toBe(NEW_TITLE);
    expect(storeApi!.getState().office.tasks.items[0]?.title).toBe(NEW_TITLE);
    expect(toast.error).not.toHaveBeenCalled();
  });
});

describe("useCommitTaskTitle write-success canonical (AC-58)", () => {
  it("records the write's own response as canonical, and a subscribeField listener observes the saved text (not the pre-write baseline) once the guard releases, with no refetch in play", async () => {
    seedInitialCanonical("t-1", "title", ORIGINAL_TITLE, TS);
    const { Wrapper } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskTitle> | null = null;
    mockUpdateTask.mockResolvedValueOnce(
      taskResponse({ title: NEW_TITLE, description: "", updated_at: WRITE_RESPONSE_TS }),
    );
    render(
      <Wrapper>
        <TitleHookProbe onReady={(c) => (commit = c)} />
      </Wrapper>,
    );

    const seen: string[] = [];
    subscribeField("t-1", "title", (value) => seen.push(value));

    await act(async () => {
      await commit!("t-1", NEW_TITLE);
    });

    expect(getCanonicalValue("t-1", "title")).toBe(NEW_TITLE);
    // The field is guarded (beginWrite) for the whole commit, so the
    // listener fires exactly once, after endWrite releases the guard, with
    // the saved value — never with the seeded pre-write baseline.
    expect(seen).toEqual([NEW_TITLE]);
  });
});

describe("useCommitTaskTitle rollback ordering", () => {
  it("restores the currently recorded canonical title on failure when no later commit is in play (AC-9, AC-64)", async () => {
    seedInitialCanonical("t-1", "title", ORIGINAL_TITLE, TS);
    const { Wrapper, state } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskTitle> | null = null;
    mockUpdateTask.mockRejectedValueOnce(new Error(WRITE_FAILURE_MESSAGE));
    render(
      <Wrapper>
        <TitleHookProbe onReady={(c) => (commit = c)} />
      </Wrapper>,
    );

    await act(async () => {
      await commit!("t-1", NEW_TITLE);
    });

    expect(state.patches).toEqual([{ title: NEW_TITLE }, { title: ORIGINAL_TITLE }]);
    expect(state.task.title).toBe(ORIGINAL_TITLE);
    expect(toast.error).toHaveBeenCalledWith(WRITE_FAILURE_MESSAGE);
  });

  it("does not restore when a later-sequenced commit already succeeded (AC-44)", async () => {
    seedInitialCanonical("t-1", "title", ORIGINAL_TITLE, TS);
    const { Wrapper, state } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskTitle> | null = null;
    render(
      <Wrapper>
        <TitleHookProbe onReady={(c) => (commit = c)} />
      </Wrapper>,
    );

    const first = deferred<ReturnType<typeof taskResponse>>();
    const second = deferred<ReturnType<typeof taskResponse>>();
    mockUpdateTask.mockReturnValueOnce(first.promise);
    mockUpdateTask.mockReturnValueOnce(second.promise);

    let p1!: Promise<void>;
    let p2!: Promise<void>;
    act(() => {
      p1 = commit!("t-1", "A");
      p2 = commit!("t-1", "B");
    });

    await act(async () => {
      second.resolve(taskResponse({ title: "B", description: "", updated_at: WRITE_RESPONSE_TS }));
      await p2;
      first.reject(new Error(WRITE_FAILURE_MESSAGE));
      await p1;
    });

    expect(state.patches).toEqual([{ title: "A" }, { title: "B" }]);
    expect(state.task.title).toBe("B");
    expect(toast.error).toHaveBeenCalledWith(WRITE_FAILURE_MESSAGE);
  });

  it("does not restore when a later-sequenced commit is still unresolved (AC-53)", async () => {
    seedInitialCanonical("t-1", "title", ORIGINAL_TITLE, TS);
    const { Wrapper, state } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskTitle> | null = null;
    render(
      <Wrapper>
        <TitleHookProbe onReady={(c) => (commit = c)} />
      </Wrapper>,
    );

    const first = deferred<ReturnType<typeof taskResponse>>();
    const second = deferred<ReturnType<typeof taskResponse>>();
    mockUpdateTask.mockReturnValueOnce(first.promise);
    mockUpdateTask.mockReturnValueOnce(second.promise);

    let p1!: Promise<void>;
    act(() => {
      p1 = commit!("t-1", "A");
      commit!("t-1", "B");
    });

    await act(async () => {
      first.reject(new Error(WRITE_FAILURE_MESSAGE));
      await p1;
    });

    // The later commit (B) is still unresolved, so the earlier failure (A)
    // must not restore anything yet.
    expect(state.patches).toEqual([{ title: "A" }, { title: "B" }]);
    expect(state.task.title).toBe("B");
    expect(toast.error).toHaveBeenCalledWith(WRITE_FAILURE_MESSAGE);

    await act(async () => {
      second.resolve(taskResponse({ title: "B", description: "", updated_at: WRITE_RESPONSE_TS }));
    });
  });
});

// AC-65's chained-double-failure regression tests live in
// use-optimistic-task-mutation-rollback-ordering.test.tsx (kept out of this
// file to stay under the max-lines limit).

describe("useCommitTaskTitle rollback field scoping", () => {
  it("restores only the title, leaving an unrelated store field changed in the meantime untouched (AC-55)", async () => {
    seedInitialCanonical("t-1", "title", ORIGINAL_TITLE, TS);
    const { Wrapper } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskTitle> | null = null;
    let storeApi: ReturnType<typeof useAppStoreApi> | null = null;
    const { promise, reject } = deferred<ReturnType<typeof taskResponse>>();
    mockUpdateTask.mockReturnValueOnce(promise);
    render(
      <Wrapper>
        <TitleHookProbe onReady={(c, s) => ([commit, storeApi] = [c, s])} />
      </Wrapper>,
    );

    let pending!: Promise<void>;
    act(() => {
      pending = commit!("t-1", NEW_TITLE);
    });

    // A second, unrelated store field changes while the title write is
    // still in flight (e.g. a concurrent priority change from elsewhere).
    act(() => {
      storeApi!.getState().patchTaskInStore("t-1", { priority: "high" });
    });

    await act(async () => {
      reject(new Error(WRITE_FAILURE_MESSAGE));
      await pending;
    });

    expect(storeApi!.getState().office.tasks.items[0]?.title).toBe(ORIGINAL_TITLE);
    expect(storeApi!.getState().office.tasks.items[0]?.priority).toBe("high");
  });
});

function DescriptionHookProbe({
  onReady,
}: {
  onReady: (commit: ReturnType<typeof useCommitTaskDescription>) => void;
}) {
  const commit = useCommitTaskDescription();
  onReady(commit);
  return null;
}

describe("useCommitTaskDescription", () => {
  it("does not patch until the save succeeds (AC-56)", async () => {
    const { Wrapper, state } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskDescription> | null = null;
    const { promise, resolve } = deferred<ReturnType<typeof taskResponse>>();
    mockUpdateTask.mockReturnValueOnce(promise);
    render(
      <Wrapper>
        <DescriptionHookProbe onReady={(c) => (commit = c)} />
      </Wrapper>,
    );

    let pending!: Promise<{ ok: true; value: string } | { ok: false }>;
    act(() => {
      pending = commit!("t-1", NEW_DESCRIPTION);
    });
    expect(state.patches).toEqual([]);
    expect(mockUpdateTask).toHaveBeenCalledWith("t-1", { description: NEW_DESCRIPTION });

    let result!: { ok: true; value: string } | { ok: false };
    await act(async () => {
      resolve(
        taskResponse({
          title: "First task",
          description: NEW_DESCRIPTION,
          updated_at: WRITE_RESPONSE_TS,
        }),
      );
      result = await pending;
    });
    expect(state.patches).toEqual([{ description: NEW_DESCRIPTION }]);
    expect(result).toEqual({ ok: true, value: NEW_DESCRIPTION });
  });

  it("leaves the draft state untouched, toasts, and reports failure (AC-18)", async () => {
    const { Wrapper, state } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskDescription> | null = null;
    mockUpdateTask.mockRejectedValueOnce(new Error("nope"));
    render(
      <Wrapper>
        <DescriptionHookProbe onReady={(c) => (commit = c)} />
      </Wrapper>,
    );

    let result!: { ok: true; value: string } | { ok: false };
    await act(async () => {
      result = await commit!("t-1", NEW_DESCRIPTION);
    });
    expect(state.patches).toEqual([]);
    expect(result).toEqual({ ok: false });
    expect(toast.error).toHaveBeenCalledWith("nope");
  });
});

describe("useCommitTaskDescription write-success canonical (AC-58)", () => {
  it("records the write's own response as canonical, and a subscribeField listener observes the saved text (not the pre-write baseline) once the guard releases, with no refetch in play", async () => {
    seedInitialCanonical("t-1", "description", "Original description", TS);
    const { Wrapper } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskDescription> | null = null;
    mockUpdateTask.mockResolvedValueOnce(
      taskResponse({
        title: baseTask.title,
        description: NEW_DESCRIPTION,
        updated_at: WRITE_RESPONSE_TS,
      }),
    );
    render(
      <Wrapper>
        <DescriptionHookProbe onReady={(c) => (commit = c)} />
      </Wrapper>,
    );

    const seen: string[] = [];
    subscribeField("t-1", "description", (value) => seen.push(value));

    await act(async () => {
      await commit!("t-1", NEW_DESCRIPTION);
    });

    expect(getCanonicalValue("t-1", "description")).toBe(NEW_DESCRIPTION);
    expect(seen).toEqual([NEW_DESCRIPTION]);
  });
});

function DescriptionHookProbeWithStore({
  onReady,
}: {
  onReady: (
    commit: ReturnType<typeof useCommitTaskDescription>,
    storeApi: ReturnType<typeof useAppStoreApi>,
  ) => void;
}) {
  const commit = useCommitTaskDescription();
  const storeApi = useAppStoreApi();
  onReady(commit, storeApi);
  return null;
}

describe("useCommitTaskDescription stale write response (P1 review fix)", () => {
  it("does not apply a write response that recordWriteSuccess rejects as stale, once a newer write already superseded it", async () => {
    seedInitialCanonical("t-1", "description", "Original description", TS);
    const { Wrapper, state } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskDescription> | null = null;
    let storeApi: ReturnType<typeof useAppStoreApi> | null = null;
    render(
      <Wrapper>
        <DescriptionHookProbeWithStore onReady={(c, s) => ([commit, storeApi] = [c, s])} />
      </Wrapper>,
    );

    const first = deferred<ReturnType<typeof taskResponse>>();
    const second = deferred<ReturnType<typeof taskResponse>>();
    mockUpdateTask.mockReturnValueOnce(first.promise);
    mockUpdateTask.mockReturnValueOnce(second.promise);

    let p1!: Promise<{ ok: true; value: string } | { ok: false }>;
    let p2!: Promise<{ ok: true; value: string } | { ok: false }>;
    act(() => {
      p1 = commit!("t-1", "Draft A");
      p2 = commit!("t-1", "Draft B");
    });

    let resultB!: { ok: true; value: string } | { ok: false };
    await act(async () => {
      second.resolve(
        taskResponse({
          title: baseTask.title,
          description: "Draft B",
          updated_at: "2026-05-02T00:00:00Z",
        }),
      );
      resultB = await p2;
    });
    expect(resultB).toEqual({ ok: true, value: "Draft B" });
    expect(state.patches).toEqual([{ description: "Draft B" }]);

    let resultA!: { ok: true; value: string } | { ok: false };
    await act(async () => {
      first.resolve(
        taskResponse({
          title: baseTask.title,
          description: "Draft A",
          updated_at: "2026-05-01T12:00:00Z",
        }),
      );
      resultA = await p1;
    });

    // Draft A's response is older than Draft B's already-recorded canonical
    // value, so recordWriteSuccess rejects it (AC-60/AC-62): it must not
    // apply to the local task or the store, and the hook must report the
    // still-current canonical value rather than the stale one it saved.
    expect(state.patches).toEqual([{ description: "Draft B" }]);
    expect(state.task.description).toBe("Draft B");
    expect(storeApi!.getState().office.tasks.items[0]?.description).toBe("Draft B");
    expect(resultA).toEqual({ ok: true, value: "Draft B" });
    expect(getCanonicalValue("t-1", "description")).toBe("Draft B");
  });
});

describe("single-field request body (AC-26)", () => {
  it("sends only the title field when committing a title, even though the local task has other properties set", async () => {
    const { Wrapper } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskTitle> | null = null;
    mockUpdateTask.mockResolvedValueOnce(
      taskResponse({ title: NEW_TITLE, description: "", updated_at: WRITE_RESPONSE_TS }),
    );
    render(
      <Wrapper>
        <TitleHookProbe onReady={(c) => (commit = c)} />
      </Wrapper>,
    );

    await act(async () => {
      await commit!("t-1", NEW_TITLE);
    });

    expect(mockUpdateTask).toHaveBeenCalledTimes(1);
    expect(mockUpdateTask.mock.calls[0]?.[1]).toEqual({ title: NEW_TITLE });
  });

  it("sends only the description field when committing a description, even though the local task has other properties set", async () => {
    const { Wrapper } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskDescription> | null = null;
    mockUpdateTask.mockResolvedValueOnce(
      taskResponse({
        title: baseTask.title,
        description: NEW_DESCRIPTION,
        updated_at: WRITE_RESPONSE_TS,
      }),
    );
    render(
      <Wrapper>
        <DescriptionHookProbe onReady={(c) => (commit = c)} />
      </Wrapper>,
    );

    await act(async () => {
      await commit!("t-1", NEW_DESCRIPTION);
    });

    expect(mockUpdateTask).toHaveBeenCalledTimes(1);
    expect(mockUpdateTask.mock.calls[0]?.[1]).toEqual({ description: NEW_DESCRIPTION });
  });
});
