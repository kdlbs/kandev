import { Suspense, startTransition, useLayoutEffect, useState, type ReactNode } from "react";
import { act, cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { TaskPR } from "@/lib/types/github";
import { ScopedTaskPRProvider } from "./scoped-task-pr-context";
import { useTaskPR } from "./use-task-pr";

type Payload = { task_id: string; workspace_id: string };
const api = vi.hoisted(() => ({
  list: vi.fn(),
  unlink: vi.fn(),
  request: vi.fn(),
  handlers: new Map<string, Set<(event: { payload: Payload }) => void>>(),
}));
vi.mock("@/lib/api/domains/github-api", () => ({
  listTaskPRs: api.list,
  deleteTaskPR: api.unlink,
  listWorkspaceTaskPRs: vi.fn(),
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => client,
  useWebSocketClient: () => client,
}));
const client = {
  request: api.request,
  on: (event: string, callback: (event: { payload: Payload }) => void) => {
    const callbacks = api.handlers.get(event) ?? new Set();
    api.handlers.set(event, callbacks);
    callbacks.add(callback);
    return () => callbacks.delete(callback);
  },
};
const pr = { id: "association", task_id: "task", workspace_id: "beta", pr_number: 42 } as TaskPR;
const globalPR = { ...pr, id: "global", task_id: "ordinary", workspace_id: "alpha" };
const response = (prs: TaskPR[]) => ({ task_prs: { task: prs } });
function Store({ children }: { children: ReactNode }) {
  return (
    <StateProvider
      initialState={{
        workspaces: { activeId: "alpha", items: [] },
        taskPRs: { byTaskId: { ordinary: [globalPR] } },
      }}
    >
      {children}
    </StateProvider>
  );
}
function mount() {
  let owner = { taskId: "task", workspaceId: "beta" };
  const hook = renderHook(() => ({ ...useTaskPR(owner.taskId), store: useAppStoreApi() }), {
    wrapper: ({ children }) => (
      <Store>
        <ScopedTaskPRProvider {...owner}>{children}</ScopedTaskPRProvider>
      </Store>
    ),
  });
  return {
    ...hook,
    switchOwner: (next: typeof owner) => {
      owner = next;
      hook.rerender();
    },
  };
}
function emit(event: string, payload: Payload) {
  act(() => api.handlers.get(event)?.forEach((callback) => callback({ payload })));
}
beforeEach(() => {
  api.handlers.clear();
  api.list.mockReset().mockResolvedValue(response([pr]));
  api.unlink.mockReset().mockResolvedValue(undefined);
  api.request.mockReset().mockResolvedValue({});
});
afterEach(cleanup);

describe("embedded original-task PR associations", () => {
  it("filters both ownership fields and never republishes into the ordinary cache", async () => {
    api.list.mockResolvedValue(
      response([
        pr,
        { ...pr, id: "foreign-task", task_id: "other" },
        { ...pr, id: "foreign-workspace", workspace_id: "alpha" },
      ]),
    );
    const { result } = mount();
    const original = result.current.store.getState().taskPRs;
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.prs).toEqual([pr]);
    expect(result.current.store.getState().taskPRs).toBe(original);
    expect(result.current.store.getState().workspaces.activeId).toBe("alpha");
    expect(api.request).not.toHaveBeenCalled();
  });
  it.each(["github.task_pr.updated", "github.task_pr.deleted"])(
    "reloads only matching %s events",
    async (event) => {
      const { result } = mount();
      await waitFor(() => expect(result.current.loaded).toBe(true));
      emit(event, { task_id: "other", workspace_id: "beta" });
      emit(event, { task_id: "task", workspace_id: "alpha" });
      expect(api.list).toHaveBeenCalledTimes(1);
      api.list.mockResolvedValueOnce(response([]));
      emit(event, { task_id: "task", workspace_id: "beta" });
      await waitFor(() => expect(result.current.prs).toEqual([]));
      expect(api.list).toHaveBeenCalledTimes(2);
    },
  );
  it("aborts an in-flight read before unlink and rejects unknown associations", async () => {
    const { result } = mount();
    await waitFor(() => expect(result.current.loaded).toBe(true));
    const pending = Promise.withResolvers<ReturnType<typeof response>>();
    api.list.mockReturnValueOnce(pending.promise);
    emit("github.task_pr.updated", { task_id: "task", workspace_id: "beta" });
    const signal = api.list.mock.calls[1][1].init.signal as AbortSignal;
    await act(async () => result.current.unlink("foreign"));
    expect(api.unlink).not.toHaveBeenCalled();
    await act(async () => result.current.unlink(pr.id));
    expect(signal.aborted).toBe(true);
    expect(api.unlink).toHaveBeenCalledWith(pr.id, "beta");
    await act(async () => pending.resolve(response([pr])));
    expect(result.current.prs).toEqual([]);
  });
});

describe("embedded PR mutations and refresh", () => {
  it("does not resurrect an association from a WS read started during unlink", async () => {
    const { result } = mount();
    await waitFor(() => expect(result.current.loaded).toBe(true));
    const deletion = Promise.withResolvers<void>();
    const pending = Promise.withResolvers<ReturnType<typeof response>>();
    api.unlink.mockReturnValueOnce(deletion.promise);
    let unlink!: Promise<void>;
    act(() => {
      unlink = result.current.unlink(pr.id);
    });
    api.list.mockReturnValueOnce(pending.promise);
    emit("github.task_pr.updated", { task_id: "task", workspace_id: "beta" });
    await act(async () => {
      deletion.resolve();
      await unlink;
    });
    expect(result.current.prs).toEqual([]);
    await act(async () => pending.resolve(response([pr])));
    expect(result.current.prs).toEqual([]);
  });
  it("refreshes independently after rejected sync while preserving that rejection", async () => {
    const { result } = mount();
    await waitFor(() => expect(result.current.loaded).toBe(true));
    const failure = new Error("sync rejected");
    api.request.mockRejectedValueOnce(failure);
    api.list.mockResolvedValueOnce(response([]));
    await act(async () => {
      await expect(result.current.refresh()).rejects.toBe(failure);
    });
    expect(api.list).toHaveBeenCalledTimes(2);
    expect(result.current.prs).toEqual([]);
    expect(api.request).toHaveBeenCalledWith("github.task_pr.sync", {
      task_id: "task",
      explicit_refresh: true,
    });
  });
  it("invalidates a pending explicit sync on unmount, including retained callbacks", async () => {
    const { result, unmount } = mount();
    await waitFor(() => expect(result.current.loaded).toBe(true));
    const sync = Promise.withResolvers<void>();
    api.request.mockReturnValueOnce(sync.promise);
    const refresh = result.current.refresh;
    const unlink = result.current.unlink;
    let pending!: Promise<void>;
    act(() => {
      pending = refresh();
    });
    unmount();
    await act(async () => {
      sync.resolve();
      await pending;
      await refresh();
      await unlink(pr.id);
    });
    expect(api.list).toHaveBeenCalledTimes(1);
    expect(api.request).toHaveBeenCalledTimes(1);
    expect(api.unlink).not.toHaveBeenCalled();
    expect([...api.handlers.values()].every((callbacks) => callbacks.size === 0)).toBe(true);
  });
});

describe("retired embedded PR read ownership", () => {
  it("does not read retired identity after a pending sync resolves", async () => {
    const { result } = mount();
    await waitFor(() => expect(result.current.loaded).toBe(true));
    const sync = Promise.withResolvers<void>();
    api.request.mockReturnValueOnce(sync.promise);
    let pending!: Promise<void>;
    act(() => {
      pending = result.current.refresh();
    });
    act(() =>
      result.current.store.setState((state) => ({
        workspaceContextGeneration: state.workspaceContextGeneration + 1,
      })),
    );
    await waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    await act(async () => {
      sync.resolve();
      await pending;
    });
    expect(api.list).toHaveBeenCalledTimes(2);
  });
  it("masks old rows on scope changes and ignores the retired read", async () => {
    const { result, switchOwner } = mount();
    await waitFor(() => expect(result.current.loaded).toBe(true));
    const pending = Promise.withResolvers<ReturnType<typeof response>>();
    api.list.mockReturnValueOnce(pending.promise);
    switchOwner({ taskId: "task", workspaceId: "gamma" });
    expect(result.current.prs).toEqual([]);
    await act(async () => pending.resolve(response([pr])));
    expect(result.current.prs).toEqual([]);
    expect(result.current.store.getState().taskPRs.byTaskId.ordinary).toEqual([globalPR]);
  });
});

it("keeps a committed read alive when a different-scope render is abandoned", async () => {
  const pending = Promise.withResolvers<ReturnType<typeof response>>();
  api.list.mockReturnValueOnce(pending.promise);
  const never = new Promise<void>(() => {});
  let change!: (workspace: string) => void;
  let committed: ReturnType<typeof useTaskPR> | undefined;
  function Probe({ suspended }: { suspended: boolean }) {
    const value = useTaskPR("task");
    useLayoutEffect(() => {
      committed = value;
    });
    if (suspended) throw never;
    return null;
  }
  function App() {
    const [workspace, setWorkspace] = useState("beta");
    change = setWorkspace;
    return (
      <Store>
        <Suspense fallback={null}>
          <ScopedTaskPRProvider taskId="task" workspaceId={workspace}>
            <Probe suspended={workspace !== "beta"} />
          </ScopedTaskPRProvider>
        </Suspense>
      </Store>
    );
  }
  render(<App />);
  expect(api.list).toHaveBeenCalledTimes(1);
  act(() => startTransition(() => change("abandoned")));
  await act(async () => pending.resolve(response([pr])));
  expect(committed?.prs).toEqual([pr]);
  expect(api.list).toHaveBeenCalledTimes(1);
});
