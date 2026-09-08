import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { TaskMRAutomationOptions, TaskMRAutomationOptionsForMR } from "@/lib/types/gitlab";

const api = vi.hoisted(() => ({
  getTaskMRAutomation: vi.fn(),
  updateTaskMRAutomation: vi.fn(),
}));

vi.mock("@/lib/api/domains/gitlab-api", () => api);

import { useTaskMRAutomationOptions } from "./use-task-mr-automation";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function defaultMROption(
  overrides: Partial<TaskMRAutomationOptionsForMR> = {},
): TaskMRAutomationOptionsForMR {
  return {
    task_id: "task-1",
    repository_id: "",
    project_path: "group/project",
    mr_iid: 7,
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    prompt_on_review_requested: false,
    prompt_on_merged: false,
    prompt_on_closed: false,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function baseOptions(overrides: Partial<TaskMRAutomationOptions> = {}): TaskMRAutomationOptions {
  return {
    task_id: "task-1",
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    auto_fix_max_rounds: 10,
    effective_auto_fix_prompt: "",
    using_default_prompt: true,
    prompt_on_review_requested: false,
    prompt_on_merged: false,
    prompt_on_closed: false,
    review_reviewer_username: "",
    updated_at: "2026-01-01T00:00:00Z",
    mr_states: [],
    mr_options: [defaultMROption()],
    ...overrides,
  };
}

const NETWORK_DOWN_ERROR = "network down";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => cleanup());

describe("useTaskMRAutomationOptions fetching", () => {
  it("fetches options on mount for a given taskId", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"), { wrapper });

    await waitFor(() => expect(result.current.options).not.toBeNull());
    expect(api.getTaskMRAutomation).toHaveBeenCalledWith("task-1", { cache: "no-store" });
    expect(result.current.options?.task_id).toBe("task-1");
    expect(result.current.error).toBeNull();
  });

  it("does not fetch when taskId is null", () => {
    renderHook(() => useTaskMRAutomationOptions(null), { wrapper });
    expect(api.getTaskMRAutomation).not.toHaveBeenCalled();
  });

  it("does not auto-retry after a load error", async () => {
    api.getTaskMRAutomation.mockRejectedValue(new Error(NETWORK_DOWN_ERROR));
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"), { wrapper });

    await waitFor(() => expect(result.current.error).toBe(NETWORK_DOWN_ERROR));
    expect(api.getTaskMRAutomation).toHaveBeenCalledTimes(1);
    await act(async () => {
      await Promise.resolve();
    });
    expect(api.getTaskMRAutomation).toHaveBeenCalledTimes(1);
  });

  it("does not re-fetch a task whose options are already in the store (WS-pushed or cached)", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions({ prompt_on_merged: true }));
    const { result, rerender } = renderHook(() => useTaskMRAutomationOptions("task-1"), {
      wrapper,
    });
    await waitFor(() => expect(result.current.options).not.toBeNull());
    expect(api.getTaskMRAutomation).toHaveBeenCalledTimes(1);

    // A remount (e.g. dropdown closed and reopened) with data already cached
    // in the store must not trigger a second fetch.
    rerender();
    await act(async () => {
      await Promise.resolve();
    });
    expect(api.getTaskMRAutomation).toHaveBeenCalledTimes(1);
  });

  it("issues one fetch when several instances mount together for the same task", async () => {
    // The switches are per-MR, so a task with N linked MRs mounts N copies of
    // MRAutomationControls — each with its own useTaskMRAutomationOptions.
    // They all read the same store slot, so they must not each fire a GET.
    const pending = deferred<TaskMRAutomationOptions>();
    api.getTaskMRAutomation.mockReturnValue(pending.promise);
    const { result } = renderHook(
      () => {
        useTaskMRAutomationOptions("task-1");
        useTaskMRAutomationOptions("task-1");
        return useTaskMRAutomationOptions("task-1");
      },
      { wrapper },
    );

    await waitFor(() => expect(api.getTaskMRAutomation).toHaveBeenCalledTimes(1));
    await act(async () => {
      pending.resolve(baseOptions({ prompt_on_merged: true }));
      await pending.promise;
    });
    await waitFor(() => expect(result.current.options?.prompt_on_merged).toBe(true));
    expect(api.getTaskMRAutomation).toHaveBeenCalledTimes(1);
  });
});

describe("useTaskMRAutomationOptions optimistic updates", () => {
  it("applies an optimistic update immediately, then reconciles with the server response", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const update = deferred<TaskMRAutomationOptions>();
    api.updateTaskMRAutomation.mockImplementation(() => update.promise);
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"), { wrapper });
    await waitFor(() => expect(result.current.options).not.toBeNull());

    act(() => {
      void result.current.update({ prompt_on_merged: true });
    });

    // Optimistic reflect happens synchronously within the update call, into
    // the targeted MR's entry in mr_options (the per-MR source of truth) —
    // not the task-level aggregate, which is server-computed.
    await waitFor(() =>
      expect(result.current.options?.mr_options?.[0]?.prompt_on_merged).toBe(true),
    );
    expect(result.current.saving).toBe(true);

    await act(async () => {
      update.resolve(baseOptions({ mr_options: [defaultMROption({ prompt_on_merged: true })] }));
    });

    await waitFor(() => expect(result.current.saving).toBe(false));
    expect(result.current.options?.mr_options?.[0]?.prompt_on_merged).toBe(true);
    expect(result.current.error).toBeNull();
  });

  it("reverts the optimistic update and surfaces an error on failure (AC27)", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const update = deferred<TaskMRAutomationOptions>();
    api.updateTaskMRAutomation.mockImplementation(() => update.promise);
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"), { wrapper });
    await waitFor(() => expect(result.current.options).not.toBeNull());

    let pending: Promise<unknown>;
    act(() => {
      pending = result.current.update({ prompt_on_closed: true });
    });
    await waitFor(() =>
      expect(result.current.options?.mr_options?.[0]?.prompt_on_closed).toBe(true),
    );

    await act(async () => {
      update.reject(new Error(NETWORK_DOWN_ERROR));
      await expect(pending!).rejects.toThrow(NETWORK_DOWN_ERROR);
    });

    expect(result.current.options?.mr_options?.[0]?.prompt_on_closed).toBe(false);
    expect(result.current.error).toBe(NETWORK_DOWN_ERROR);
    expect(result.current.saving).toBe(false);
  });
});

describe("useTaskMRAutomationOptions races", () => {
  it("does not let a concurrent refresh discard an update's response (independent generations)", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const update = deferred<TaskMRAutomationOptions>();
    api.updateTaskMRAutomation.mockImplementation(() => update.promise);
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"), { wrapper });
    await waitFor(() => expect(result.current.options).not.toBeNull());

    // Start an update (PATCH in flight)...
    act(() => {
      void result.current.update({ prompt_on_merged: true });
    });
    await waitFor(() => expect(result.current.saving).toBe(true));

    // ...then a refresh races in and resolves first, with a pre-patch
    // snapshot (server hasn't processed the PATCH yet).
    const refresh = deferred<TaskMRAutomationOptions>();
    api.getTaskMRAutomation.mockImplementation(() => refresh.promise);
    act(() => {
      void result.current.refresh();
    });
    await act(async () => {
      refresh.resolve(baseOptions({ prompt_on_merged: false }));
    });

    // The update's response must still land — a refresh racing in must not
    // permanently suppress it.
    await act(async () => {
      update.resolve(baseOptions({ prompt_on_merged: true }));
    });
    await waitFor(() => expect(result.current.saving).toBe(false));
    expect(result.current.options?.prompt_on_merged).toBe(true);
  });

  it("does not let a refresh started before a save overwrite the saved result", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"), { wrapper });
    await waitFor(() => expect(result.current.options).not.toBeNull());

    // A manual refresh starts and stays in flight...
    const refresh = deferred<TaskMRAutomationOptions>();
    api.getTaskMRAutomation.mockReturnValueOnce(refresh.promise);
    act(() => {
      void result.current.refresh();
    });

    // ...while a save starts and completes first.
    api.updateTaskMRAutomation.mockResolvedValue(baseOptions({ prompt_on_merged: true }));
    await act(async () => {
      await result.current.update({ prompt_on_merged: true });
    });
    expect(result.current.options?.prompt_on_merged).toBe(true);

    // The stale refresh (started before the save) now resolves with
    // pre-save data — it must not flip the saved switch back off.
    await act(async () => {
      refresh.resolve(baseOptions({ prompt_on_merged: false }));
    });
    expect(result.current.options?.prompt_on_merged).toBe(true);
  });

  // A task with N linked MRs mounts N MRAutomationControls, each with its own
  // useTaskMRAutomationOptions("task-1") instance. All instances read/write
  // the same store slot, so their save-ordering guards must also be shared —
  // otherwise an older instance's private counter never learns about a
  // newer instance's save and treats its own stale response as current.
  it("shares update ordering across multiple mounted instances for the same task, so an older instance's stale response cannot clobber a newer one's committed result", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const { result } = renderHook(
      () => ({
        a: useTaskMRAutomationOptions("task-1"),
        b: useTaskMRAutomationOptions("task-1"),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.a.options).not.toBeNull());

    const updateA = deferred<TaskMRAutomationOptions>();
    const updateB = deferred<TaskMRAutomationOptions>();
    api.updateTaskMRAutomation
      .mockImplementationOnce(() => updateA.promise)
      .mockImplementationOnce(() => updateB.promise);

    // Instance A (e.g. !1's block) starts a save...
    act(() => {
      void result.current.a.update({ auto_fix_prompt_override: "from A" });
    });
    // ...then instance B (e.g. !2's block) starts a later save for the same
    // task before A's resolves.
    act(() => {
      void result.current.b.update({ auto_fix_prompt_override: "from B" });
    });

    // B's later, authoritative response lands first...
    await act(async () => {
      updateB.resolve(baseOptions({ auto_fix_prompt_override: "from B" }));
    });
    expect(result.current.a.options?.auto_fix_prompt_override).toBe("from B");

    // ...then A's now-stale response resolves after. It must not win.
    await act(async () => {
      updateA.resolve(baseOptions({ auto_fix_prompt_override: "from A" }));
    });
    expect(result.current.a.options?.auto_fix_prompt_override).toBe("from B");
    expect(result.current.b.options?.auto_fix_prompt_override).toBe("from B");
  });
});

describe("useTaskMRAutomationOptions revision ordering", () => {
  it("commits a higher-revision response even when its request started before a lower-revision response", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions({ automation_revision: 1 }));
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"), { wrapper });
    await waitFor(() => expect(result.current.options).not.toBeNull());

    const olderRequest = deferred<TaskMRAutomationOptions>();
    const newerRequest = deferred<TaskMRAutomationOptions>();
    api.updateTaskMRAutomation
      .mockImplementationOnce(() => olderRequest.promise)
      .mockImplementationOnce(() => newerRequest.promise);

    act(() => {
      void result.current.update({ auto_fix_prompt_override: "older request" });
      void result.current.update({ auto_fix_prompt_override: "newer request" });
    });

    await act(async () => {
      newerRequest.resolve(
        baseOptions({ automation_revision: 2, auto_fix_prompt_override: "revision two" }),
      );
    });
    expect(result.current.options?.automation_revision).toBe(2);

    // The first request can commit later when the database transaction that
    // owns it receives the next revision. Request-start order is not server
    // revision order, so the higher revision must still win.
    await act(async () => {
      olderRequest.resolve(
        baseOptions({ automation_revision: 3, auto_fix_prompt_override: "revision three" }),
      );
    });
    expect(result.current.options?.automation_revision).toBe(3);
    expect(result.current.options?.auto_fix_prompt_override).toBe("revision three");
  });
});

describe("useTaskMRAutomationOptions task switching", () => {
  it("keeps each task's options in its own store slot — a stale response for one task cannot leak into another", async () => {
    const taskA = deferred<TaskMRAutomationOptions>();
    api.getTaskMRAutomation.mockImplementation((taskId: string) =>
      taskId === "task-a" ? taskA.promise : Promise.resolve(baseOptions({ task_id: "task-b" })),
    );

    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string | null }) => useTaskMRAutomationOptions(taskId),
      { wrapper, initialProps: { taskId: "task-a" } },
    );

    // task-a's fetch is still in flight when the caller switches to task-b.
    rerender({ taskId: "task-b" });
    await waitFor(() => expect(result.current.options?.task_id).toBe("task-b"));

    // task-a's stale response resolves after the switch — it lands in
    // task-a's own store slot, not task-b's currently-displayed options.
    await act(async () => {
      taskA.resolve(baseOptions({ task_id: "task-a" }));
    });
    expect(result.current.options?.task_id).toBe("task-b");
  });

  it("shows a revisited task's already-cached options immediately without an extra fetch", async () => {
    api.getTaskMRAutomation.mockResolvedValueOnce(baseOptions({ task_id: "task-a" }));
    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string | null }) => useTaskMRAutomationOptions(taskId),
      { wrapper, initialProps: { taskId: "task-a" } },
    );
    await waitFor(() => expect(result.current.options?.task_id).toBe("task-a"));

    api.getTaskMRAutomation.mockResolvedValueOnce(baseOptions({ task_id: "task-b" }));
    rerender({ taskId: "task-b" });
    await waitFor(() => expect(result.current.options?.task_id).toBe("task-b"));

    // Switching back to task-a must show its cached slot immediately,
    // without issuing a third fetch.
    rerender({ taskId: "task-a" });
    expect(result.current.options?.task_id).toBe("task-a");
    expect(api.getTaskMRAutomation).toHaveBeenCalledTimes(2);
  });

  it("clears loading for the original task when switching tasks mid-refresh", async () => {
    let resolveFirst: (value: TaskMRAutomationOptions) => void = () => {};
    let resolveSecond: (value: TaskMRAutomationOptions) => void = () => {};
    api.getTaskMRAutomation
      .mockReturnValueOnce(new Promise((resolve) => (resolveFirst = resolve)))
      .mockReturnValueOnce(new Promise((resolve) => (resolveSecond = resolve)));

    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string | null }) => ({
        hook: useTaskMRAutomationOptions(taskId),
        automation: useAppStore((state) => state.taskMRAutomation),
      }),
      { wrapper, initialProps: { taskId: "task-a" } },
    );

    await waitFor(() => expect(result.current.automation.loading["task-a"]).toBe(true));
    rerender({ taskId: "task-b" });
    await waitFor(() => expect(result.current.automation.loading["task-b"]).toBe(true));

    resolveFirst(baseOptions({ task_id: "task-a" }));
    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.automation.loading["task-a"]).toBe(false);
    expect(result.current.automation.loading["task-b"]).toBe(true);

    resolveSecond(baseOptions({ task_id: "task-b" }));
    await waitFor(() => expect(result.current.automation.loading["task-b"]).toBe(false));
  });
});

describe("useTaskMRAutomationOptions live updates", () => {
  it("reflects a WS-pushed options update (e.g. from another tab or MCP) without a fetch", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const { result } = renderHook(
      () => ({
        hook: useTaskMRAutomationOptions("task-1"),
        setOptions: useAppStore((state) => state.setTaskMRAutomationOptions),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.hook.options).not.toBeNull());
    expect(api.getTaskMRAutomation).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.setOptions("task-1", baseOptions({ prompt_on_merged: true }));
    });

    expect(result.current.hook.options?.prompt_on_merged).toBe(true);
    // The push updates the store directly — no extra fetch triggered by it.
    expect(api.getTaskMRAutomation).toHaveBeenCalledTimes(1);
  });

  it("does not let a slower update response overwrite a WS push that landed while it was in flight", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const update = deferred<TaskMRAutomationOptions>();
    api.updateTaskMRAutomation.mockImplementation(() => update.promise);
    const { result } = renderHook(
      () => ({
        hook: useTaskMRAutomationOptions("task-1"),
        setOptions: useAppStore((state) => state.setTaskMRAutomationOptions),
        markExternal: useAppStore((state) => state.markTaskMRAutomationExternalUpdate),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.hook.options).not.toBeNull());

    act(() => {
      void result.current.hook.update({ prompt_on_merged: true });
    });
    await waitFor(() => expect(result.current.hook.saving).toBe(true));

    // A push from another tab/MCP lands while the PATCH is still in flight.
    act(() => {
      result.current.setOptions("task-1", baseOptions({ prompt_on_closed: true }));
      result.current.markExternal("task-1");
    });

    // The PATCH's own (now-stale) response must not clobber the push.
    await act(async () => {
      update.resolve(baseOptions({ prompt_on_merged: true }));
    });
    expect(result.current.hook.options?.prompt_on_closed).toBe(true);
    expect(result.current.hook.options?.prompt_on_merged).toBe(false);
  });
});

describe("useTaskMRAutomationOptions rollback vs WS push", () => {
  it("does not let a failed update's rollback overwrite a WS push that landed while it was in flight", async () => {
    api.getTaskMRAutomation.mockResolvedValue(baseOptions());
    const update = deferred<TaskMRAutomationOptions>();
    api.updateTaskMRAutomation.mockImplementation(() => update.promise);
    const { result } = renderHook(
      () => ({
        hook: useTaskMRAutomationOptions("task-1"),
        setOptions: useAppStore((state) => state.setTaskMRAutomationOptions),
        markExternal: useAppStore((state) => state.markTaskMRAutomationExternalUpdate),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.hook.options).not.toBeNull());

    let updateCall: Promise<TaskMRAutomationOptions | null>;
    act(() => {
      updateCall = result.current.hook.update({ prompt_on_merged: true });
    });
    await waitFor(() => expect(result.current.hook.saving).toBe(true));

    act(() => {
      result.current.setOptions("task-1", baseOptions({ prompt_on_closed: true }));
      result.current.markExternal("task-1");
    });

    await act(async () => {
      update.reject(new Error(NETWORK_DOWN_ERROR));
      await expect(updateCall).rejects.toThrow(NETWORK_DOWN_ERROR);
    });
    // The rollback must not erase the pushed value — only the error surfaces.
    expect(result.current.hook.options?.prompt_on_closed).toBe(true);
    expect(result.current.hook.error).toBe(NETWORK_DOWN_ERROR);
  });

  it("does not let a slower refresh response overwrite a WS push that landed while it was in flight", async () => {
    const refreshCall = deferred<TaskMRAutomationOptions>();
    api.getTaskMRAutomation
      .mockResolvedValueOnce(baseOptions())
      .mockReturnValueOnce(refreshCall.promise);
    const { result } = renderHook(
      () => ({
        hook: useTaskMRAutomationOptions("task-1"),
        setOptions: useAppStore((state) => state.setTaskMRAutomationOptions),
        markExternal: useAppStore((state) => state.markTaskMRAutomationExternalUpdate),
      }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.hook.options).not.toBeNull());

    act(() => {
      void result.current.hook.refresh();
    });

    act(() => {
      result.current.setOptions("task-1", baseOptions({ prompt_on_closed: true }));
      result.current.markExternal("task-1");
    });

    await act(async () => {
      refreshCall.resolve(baseOptions({ prompt_on_merged: true }));
    });
    expect(result.current.hook.options?.prompt_on_closed).toBe(true);
    expect(result.current.hook.options?.prompt_on_merged).toBe(false);
  });
});

describe("useTaskMRAutomationOptions refresh/save interaction", () => {
  it("does not let a refresh in flight during a pending save flip the optimistic switch back off", async () => {
    const refreshCall = deferred<TaskMRAutomationOptions>();
    api.getTaskMRAutomation
      .mockResolvedValueOnce(baseOptions())
      .mockReturnValueOnce(refreshCall.promise);
    const update = deferred<TaskMRAutomationOptions>();
    api.updateTaskMRAutomation.mockImplementation(() => update.promise);
    const { result } = renderHook(() => useTaskMRAutomationOptions("task-1"), { wrapper });
    await waitFor(() => expect(result.current.options).not.toBeNull());

    act(() => {
      void result.current.update({ prompt_on_merged: true });
    });
    await waitFor(() => expect(result.current.saving).toBe(true));

    act(() => {
      void result.current.refresh();
    });
    // The refresh's pre-patch response resolves while the save is still
    // pending — it must not flip the optimistic switch back off.
    await act(async () => {
      refreshCall.resolve(
        baseOptions({ mr_options: [defaultMROption({ prompt_on_merged: false })] }),
      );
    });
    expect(result.current.options?.mr_options?.[0]?.prompt_on_merged).toBe(true);

    await act(async () => {
      update.resolve(baseOptions({ mr_options: [defaultMROption({ prompt_on_merged: true })] }));
    });
    expect(result.current.options?.mr_options?.[0]?.prompt_on_merged).toBe(true);
  });
});
