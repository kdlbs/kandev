import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { TaskOptimisticContextProvider, useCommitTaskTitle } from "./use-optimistic-task-mutation";
import type { Task } from "@/app/office/tasks/[id]/types";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import type { Task as HttpTask } from "@/lib/types/http";
import {
  __resetOfficeTaskContentSyncForTests,
  seedInitialCanonical,
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
