import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render } from "@testing-library/react";
import { useMemo, useState, type ReactNode } from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import {
  TaskOptimisticContextProvider,
  useCommitTaskTitle,
  useOptimisticTaskMutation,
} from "./use-optimistic-task-mutation";
import type { Task } from "@/app/office/tasks/[id]/types";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import type { Task as HttpTask } from "@/lib/types/http";
import {
  __resetOfficeTaskContentSyncForTests,
  seedInitialCanonical,
} from "@/lib/state/office-task-content-sync";
import { ApprovalGateError } from "@/lib/api/domains/office-status-gate";

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
const ORIGINAL_TITLE = "Original Title";
const WRITE_FAILURE_MESSAGE = "boom";

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

function makeStoreSeed(initialOffice: OfficeTask | null) {
  return function StoreSeed({ children }: { children: ReactNode }) {
    const api = useAppStoreApi();
    if (initialOffice) {
      api.getState().setTasks([initialOffice]);
    }
    return <>{children}</>;
  };
}

function makeHarness(initialTask: Task, initialOffice: OfficeTask | null) {
  const state = {
    task: initialTask,
    patches: [] as Partial<Task>[],
  };
  const ctxValue = {
    task: initialTask,
    applyPatch: (patch: Partial<Task>) => {
      state.patches.push(patch);
      state.task = { ...state.task, ...patch };
    },
    restore: (snapshot: Task) => {
      state.task = snapshot;
    },
  };
  const StoreSeed = makeStoreSeed(initialOffice);
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

/**
 * Unlike `makeHarness`, `ctx.task` here is backed by real React state so it
 * reflects each mutate call's actual pre-patch board state (including a
 * still-unresolved earlier call's optimistic patch) instead of a snapshot
 * frozen at harness creation. The generic mutation hook's rollback restores
 * to `ctx.task` captured at issue time, so an interleaving test needs that
 * value to behave the way a real page component's props would.
 */
function makeLiveHarness(initialTask: Task, initialOffice: OfficeTask | null) {
  const taskRef: { current: Task } = { current: initialTask };
  const mutateRef: { current: ReturnType<typeof useOptimisticTaskMutation> | null } = {
    current: null,
  };
  const storeApiRef: { current: ReturnType<typeof useAppStoreApi> | null } = { current: null };

  function Inner() {
    const [task, setTask] = useState(initialTask);
    taskRef.current = task;
    const ctxValue = useMemo(
      () => ({
        task,
        applyPatch: (patch: Partial<Task>) => setTask((t) => ({ ...t, ...patch })),
        restore: (snapshot: Task) => setTask(snapshot),
      }),
      [task],
    );
    return (
      <TaskOptimisticContextProvider value={ctxValue}>
        <GenericMutationProbe
          onReady={(m, s) => {
            mutateRef.current = m;
            storeApiRef.current = s;
          }}
        />
      </TaskOptimisticContextProvider>
    );
  }

  const StoreSeed = makeStoreSeed(initialOffice);

  function Wrapper() {
    return (
      <StateProvider>
        <StoreSeed>
          <Inner />
        </StoreSeed>
      </StateProvider>
    );
  }

  return { Wrapper, taskRef, mutateRef, storeApiRef };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

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

function GenericMutationProbe({
  onReady,
}: {
  onReady: (
    mutate: ReturnType<typeof useOptimisticTaskMutation>,
    storeApi: ReturnType<typeof useAppStoreApi>,
  ) => void;
}) {
  const mutate = useOptimisticTaskMutation();
  const storeApi = useAppStoreApi();
  onReady(mutate, storeApi);
  return null;
}

describe("useCommitTaskTitle rollback ordering — AC-65 chained failures", () => {
  it("chained double failure: restores only once both commits have resolved, regardless of resolution order (AC-65)", async () => {
    seedInitialCanonical("t-1", "title", ORIGINAL_TITLE, TS);
    const { Wrapper, state } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskTitle> | null = null;
    let storeApi: ReturnType<typeof useAppStoreApi> | null = null;
    render(
      <Wrapper>
        <TitleHookProbe onReady={(c, s) => ([commit, storeApi] = [c, s])} />
      </Wrapper>,
    );

    const first = deferred<HttpTask>();
    const second = deferred<HttpTask>();
    mockUpdateTask.mockReturnValueOnce(first.promise);
    mockUpdateTask.mockReturnValueOnce(second.promise);

    let p1!: Promise<void>;
    let p2!: Promise<void>;
    act(() => {
      p1 = commit!("t-1", "A");
      p2 = commit!("t-1", "B");
    });

    // AC-53: the earlier commit (A) fails first, but B is still unresolved,
    // so no restore happens yet — the displayed title stays at B's draft.
    await act(async () => {
      first.reject(new Error(WRITE_FAILURE_MESSAGE));
      await p1;
    });
    expect(state.task.title).toBe("B");
    expect(storeApi!.getState().office.tasks.items[0]?.title).toBe("B");

    // AC-65: the later commit (B) now also fails. Nothing is left pending,
    // so this is the restore that actually fires — to the recorded canonical
    // value, not to either commit's draft. Assert both the displayed title
    // and the Office task store entry, per the spec's explicit "both end at
    // the canonical title" requirement for this regression test.
    await act(async () => {
      second.reject(new Error(WRITE_FAILURE_MESSAGE));
      await p2;
    });
    expect(state.task.title).toBe(ORIGINAL_TITLE);
    expect(storeApi!.getState().office.tasks.items[0]?.title).toBe(ORIGINAL_TITLE);
    expect(state.patches).toEqual([{ title: "A" }, { title: "B" }, { title: ORIGINAL_TITLE }]);
    expect(toast.error).toHaveBeenCalledTimes(2);
  });

  it("chained double failure: restores as soon as the later commit resolves, even if it fails first (AC-65)", async () => {
    seedInitialCanonical("t-1", "title", ORIGINAL_TITLE, TS);
    const { Wrapper, state } = makeHarness(baseTask, baseOfficeTask);
    let commit: ReturnType<typeof useCommitTaskTitle> | null = null;
    let storeApi: ReturnType<typeof useAppStoreApi> | null = null;
    render(
      <Wrapper>
        <TitleHookProbe onReady={(c, s) => ([commit, storeApi] = [c, s])} />
      </Wrapper>,
    );

    const first = deferred<HttpTask>();
    const second = deferred<HttpTask>();
    mockUpdateTask.mockReturnValueOnce(first.promise);
    mockUpdateTask.mockReturnValueOnce(second.promise);

    let p1!: Promise<void>;
    let p2!: Promise<void>;
    act(() => {
      p1 = commit!("t-1", "A");
      p2 = commit!("t-1", "B");
    });

    // The later commit (B) fails first, and nothing later is pending, so it
    // restores immediately to canonical.
    await act(async () => {
      second.reject(new Error(WRITE_FAILURE_MESSAGE));
      await p2;
    });
    expect(state.task.title).toBe(ORIGINAL_TITLE);
    expect(storeApi!.getState().office.tasks.items[0]?.title).toBe(ORIGINAL_TITLE);

    // The earlier commit (A) then also fails. B already resolved, so this
    // restore fires too (per AC-65's "restore in either resolution order"),
    // converging on the same canonical value rather than clobbering it —
    // asserted on both the displayed title and the Office task store entry
    // so this reaches the same end state as the reverse resolution order
    // above regardless of which layer is read.
    await act(async () => {
      first.reject(new Error(WRITE_FAILURE_MESSAGE));
      await p1;
    });
    expect(state.task.title).toBe(ORIGINAL_TITLE);
    expect(storeApi!.getState().office.tasks.items[0]?.title).toBe(ORIGINAL_TITLE);
    expect(state.patches).toEqual([
      { title: "A" },
      { title: "B" },
      { title: ORIGINAL_TITLE },
      { title: ORIGINAL_TITLE },
    ]);
    expect(toast.error).toHaveBeenCalledTimes(2);
  });
});

describe("useOptimisticTaskMutation generation guard — overlapping status mutations", () => {
  it("older-fails-after-newer-succeeds: the older failure's rollback does not clobber the newer, server-confirmed state, but it still toasts and rejects", async () => {
    const { Wrapper, taskRef, storeApiRef, mutateRef } = makeLiveHarness(baseTask, baseOfficeTask);
    render(<Wrapper />);

    const older = deferred<void>();
    const newer = deferred<void>();
    let p1!: Promise<void>;
    let p2!: Promise<void>;

    // The older mutation issues first (todo -> in_progress); the newer one
    // issues while it is still pending (in_progress -> blocked), matching two
    // fast consecutive board drags on the same card.
    act(() => {
      p1 = mutateRef.current!("t-1", { status: "in_progress" }, () => older.promise);
    });
    act(() => {
      p2 = mutateRef.current!("t-1", { status: "blocked" }, () => newer.promise);
    });
    const p1Settled = p1.catch(() => undefined);

    // The newer mutation resolves first and succeeds.
    await act(async () => {
      newer.resolve();
      await p2;
    });
    expect(taskRef.current.status).toBe("blocked");
    expect(storeApiRef.current!.getState().office.tasks.items[0]?.status).toBe("blocked");

    // The older mutation then fails. Its rollback must be suppressed: the
    // newer, already-succeeded status must survive.
    await act(async () => {
      older.reject(new Error(WRITE_FAILURE_MESSAGE));
      await p1Settled;
    });
    expect(taskRef.current.status).toBe("blocked");
    expect(storeApiRef.current!.getState().office.tasks.items[0]?.status).toBe("blocked");
    await expect(p1).rejects.toThrow(WRITE_FAILURE_MESSAGE);
    expect(toast.error).toHaveBeenCalledTimes(1);
  });

  it("chained double failure: the newer mutation restores its own pre-patch snapshot first, then the older's own resolution converges the board back to the true baseline", async () => {
    const { Wrapper, taskRef, storeApiRef, mutateRef } = makeLiveHarness(baseTask, baseOfficeTask);
    render(<Wrapper />);

    const older = deferred<void>();
    const newer = deferred<void>();
    let p1!: Promise<void>;
    let p2!: Promise<void>;

    act(() => {
      p1 = mutateRef.current!("t-1", { status: "in_progress" }, () => older.promise);
    });
    act(() => {
      p2 = mutateRef.current!("t-1", { status: "blocked" }, () => newer.promise);
    });
    const p1Settled = p1.catch(() => undefined);
    const p2Settled = p2.catch(() => undefined);

    // The newer mutation fails first while the older is still pending: it is
    // the last word standing, so it restores — but only to its own captured
    // snapshot (the board state right before its own patch, i.e. still
    // showing the older mutation's still-unresolved optimistic status).
    await act(async () => {
      newer.reject(new Error(WRITE_FAILURE_MESSAGE));
      await p2Settled;
    });
    expect(taskRef.current.status).toBe("in_progress");
    expect(storeApiRef.current!.getState().office.tasks.items[0]?.status).toBe("in_progress");

    // The older mutation then also fails. Nothing is pending ahead of it any
    // longer, so its own restore now fires too, converging the board on the
    // true pre-mutation baseline.
    await act(async () => {
      older.reject(new Error(WRITE_FAILURE_MESSAGE));
      await p1Settled;
    });
    expect(taskRef.current.status).toBe("todo");
    expect(storeApiRef.current!.getState().office.tasks.items[0]?.status).toBe("todo");
    expect(toast.error).toHaveBeenCalledTimes(2);
  });

  it("gate-redirect superseded by a newer success does not clobber the newer status", async () => {
    const { Wrapper, taskRef, storeApiRef, mutateRef } = makeLiveHarness(baseTask, baseOfficeTask);
    render(<Wrapper />);

    const older = deferred<void>();
    const newer = deferred<void>();
    let p1!: Promise<void>;
    let p2!: Promise<void>;

    // The older mutation (todo -> done) hits the approver gate; the newer one
    // (in_progress -> blocked) issues while it is still pending and succeeds
    // first, matching a status pick immediately followed by a board drag.
    act(() => {
      p1 = mutateRef.current!("t-1", { status: "done" }, () => older.promise);
    });
    act(() => {
      p2 = mutateRef.current!("t-1", { status: "blocked" }, () => newer.promise);
    });
    const p1Settled = p1.catch(() => undefined);

    await act(async () => {
      newer.resolve();
      await p2;
    });
    expect(taskRef.current.status).toBe("blocked");
    expect(storeApiRef.current!.getState().office.tasks.items[0]?.status).toBe("blocked");

    // The older mutation's gate redirect resolves after the newer mutation
    // already succeeded: settling on "in_review" here would clobber the
    // newer, server-confirmed "blocked" status the same way an unconditional
    // rollback would.
    const gate = "Cannot mark done: awaiting approval from Ada, Grace";
    await act(async () => {
      older.reject(new ApprovalGateError(gate, "in_review"));
      await p1Settled;
    });
    expect(taskRef.current.status).toBe("blocked");
    expect(storeApiRef.current!.getState().office.tasks.items[0]?.status).toBe("blocked");
    await expect(p1).rejects.toThrow(gate);
    expect(toast.error).toHaveBeenCalledTimes(1);
  });
});
