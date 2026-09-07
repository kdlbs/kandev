import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";
import type { TaskCIAutomationOptions } from "@/lib/types/github";

const listTaskPRsMock = vi.hoisted(() => vi.fn());
const getTaskCIAutomationOptionsMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/github-api", () => ({
  listTaskPRs: listTaskPRsMock,
  getTaskCIAutomationOptions: getTaskCIAutomationOptionsMock,
}));

import { useTaskPRTooltipHydration } from "./use-task-pr-tooltip-hydration";

const WORKSPACE_A = "workspace-a";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-1",
    workspace_id: WORKSPACE_A,
    task_id: "task-1",
    repository_id: "repository-1",
    owner: "acme",
    repo: "demo",
    pr_number: 7,
    pr_url: "https://github.com/acme/demo/pull/7",
    pr_title: "Hydrate this pull request",
    head_branch: "feature/hydration",
    base_branch: "main",
    author_login: "octocat",
    state: "open",
    review_state: "approved",
    checks_state: "success",
    mergeable_state: "clean",
    review_count: 1,
    pending_review_count: 0,
    required_reviews: 1,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 1,
    checks_passing: 1,
    additions: 1,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
    ...overrides,
  };
}

function makeAutomationOptions(
  overrides: Partial<TaskCIAutomationOptions> = {},
): TaskCIAutomationOptions {
  return {
    task_id: "task-1",
    workspace_id: WORKSPACE_A,
    auto_fix_enabled: true,
    auto_merge_enabled: false,
    auto_fix_prompt_override: null,
    effective_auto_fix_prompt: "",
    using_default_prompt: true,
    updated_at: "2026-08-01T00:00:00Z",
    pr_states: [],
    pr_options: [],
    ...overrides,
  };
}

function createStateWrapper(initialState: Partial<AppState> = {}) {
  return function StateTestWrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider
        initialState={{
          workspaces: { items: [], activeId: WORKSPACE_A },
          ...initialState,
        }}
      >
        {children}
      </StateProvider>
    );
  };
}

function useHydrationWithStore(taskId: string) {
  return {
    hydration: useTaskPRTooltipHydration(taskId),
    store: useAppStoreApi(),
  };
}

beforeEach(() => {
  listTaskPRsMock.mockReset();
  getTaskCIAutomationOptionsMock.mockReset();
});

afterEach(() => {
  cleanup();
});

describe("useTaskPRTooltipHydration request coordination", () => {
  it("keeps its aggregate result stable when inputs are unchanged", () => {
    const { result, rerender } = renderHook(() => useTaskPRTooltipHydration("task-1"), {
      wrapper: createStateWrapper(),
    });
    const firstResult = result.current;

    rerender();

    expect(result.current).toBe(firstResult);
  });

  it("deduplicates concurrent requests for the same task and store", async () => {
    const response = deferred<{ task_prs: Record<string, TaskPR[]> }>();
    listTaskPRsMock.mockReturnValue(response.promise);

    const { result } = renderHook(
      () => {
        const first = useTaskPRTooltipHydration("task-1");
        const second = useTaskPRTooltipHydration("task-1");
        return { first, second, store: useAppStoreApi() };
      },
      { wrapper: createStateWrapper() },
    );

    expect(listTaskPRsMock).not.toHaveBeenCalled();
    let firstRequest!: Promise<unknown>;
    await act(async () => {
      firstRequest = result.current.first.hydrate();
      void result.current.second.hydrate();
    });

    expect(listTaskPRsMock).toHaveBeenCalledTimes(1);
    expect(listTaskPRsMock).toHaveBeenCalledWith(["task-1"], { cache: "no-store" });

    const pr = makePR();
    await act(async () => {
      response.resolve({ task_prs: { "task-1": [pr] } });
      await firstRequest;
    });

    await waitFor(() =>
      expect(result.current.store.getState().taskPRs.byTaskId["task-1"]).toEqual([pr]),
    );
  });

  it("uses the settled store entry without making another request", async () => {
    const pr = makePR();
    listTaskPRsMock.mockResolvedValue({ task_prs: { "task-1": [pr] } });

    const { result } = renderHook(() => useHydrationWithStore("task-1"), {
      wrapper: createStateWrapper(),
    });

    await act(async () => {
      await result.current.hydration.hydrate();
      await result.current.hydration.hydrate();
    });

    expect(listTaskPRsMock).toHaveBeenCalledTimes(1);
    expect(result.current.store.getState().taskPRs.byTaskId["task-1"]).toEqual([pr]);
  });
});

describe("useTaskPRTooltipHydration automation details", () => {
  it("hydrates PR details and automation options together on demand", async () => {
    const pr = makePR();
    listTaskPRsMock.mockResolvedValue({ task_prs: { "task-1": [pr] } });
    const options = makeAutomationOptions();
    getTaskCIAutomationOptionsMock.mockResolvedValue(options);

    const { result } = renderHook(
      () => useTaskPRTooltipHydration("task-1", { includeAutomation: true }),
      { wrapper: createStateWrapper() },
    );

    expect(listTaskPRsMock).not.toHaveBeenCalled();
    expect(getTaskCIAutomationOptionsMock).not.toHaveBeenCalled();
    await act(async () => {
      await result.current.hydrate();
    });

    expect(listTaskPRsMock).toHaveBeenCalledWith(["task-1"], { cache: "no-store" });
    expect(getTaskCIAutomationOptionsMock).toHaveBeenCalledWith("task-1", { cache: "no-store" });
    expect(result.current.automationOptions).toEqual(options);
  });

  it("uses a newer WebSocket automation update while the disclosure stays mounted", async () => {
    const pr = makePR();
    listTaskPRsMock.mockResolvedValue({ task_prs: { "task-1": [pr] } });
    const initial = makeAutomationOptions();
    getTaskCIAutomationOptionsMock.mockResolvedValue(initial);

    const { result } = renderHook(
      () => ({
        hydration: useTaskPRTooltipHydration("task-1", { includeAutomation: true }),
        store: useAppStoreApi(),
      }),
      { wrapper: createStateWrapper() },
    );

    await act(async () => {
      await result.current.hydration.hydrate();
    });

    const updated = makeAutomationOptions({
      auto_merge_enabled: true,
      updated_at: "2026-08-01T00:01:00Z",
    });
    act(() => {
      result.current.store.getState().setTaskCIAutomationOptions("task-1", updated);
    });

    await waitFor(() => expect(result.current.hydration.automationOptions).toEqual(updated));
  });
});

describe("useTaskPRTooltipHydration context guards", () => {
  it("keeps a WebSocket entry when the older HTTP response resolves later", async () => {
    const response = deferred<{ task_prs: Record<string, TaskPR[]> }>();
    listTaskPRsMock.mockReturnValue(response.promise);
    const { result } = renderHook(() => useHydrationWithStore("task-1"), {
      wrapper: createStateWrapper(),
    });
    const websocketPR = makePR({ pr_title: "Fresh WebSocket title", author_login: "fresh-user" });
    const httpPR = makePR({ pr_title: "Stale HTTP title", author_login: "stale-user" });

    let request!: Promise<unknown>;
    await act(async () => {
      request = result.current.hydration.hydrate();
      result.current.store.getState().setTaskPR("task-1", websocketPR);
    });
    await act(async () => {
      response.resolve({ task_prs: { "task-1": [httpPR] } });
      await request;
    });

    expect(result.current.store.getState().taskPRs.byTaskId["task-1"]).toEqual([websocketPR]);
  });

  it("does not apply a response after the active workspace changes", async () => {
    const response = deferred<{ task_prs: Record<string, TaskPR[]> }>();
    listTaskPRsMock.mockReturnValue(response.promise);
    const { result } = renderHook(() => useHydrationWithStore("task-1"), {
      wrapper: createStateWrapper(),
    });
    const request = result.current.hydration.hydrate();

    await act(async () => {
      result.current.store.getState().setActiveWorkspace("workspace-b");
      response.resolve({ task_prs: { "task-1": [makePR()] } });
      await request;
    });

    expect(result.current.store.getState().taskPRs.byTaskId["task-1"]).toBeUndefined();
  });

  it("does not apply a response after the task id changes", async () => {
    const response = deferred<{ task_prs: Record<string, TaskPR[]> }>();
    listTaskPRsMock.mockReturnValue(response.promise);
    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string }) => useHydrationWithStore(taskId),
      {
        initialProps: { taskId: "task-1" },
        wrapper: createStateWrapper(),
      },
    );
    const request = result.current.hydration.hydrate();

    rerender({ taskId: "task-2" });
    await act(async () => {
      response.resolve({ task_prs: { "task-1": [makePR()] } });
      await request;
    });

    expect(result.current.store.getState().taskPRs.byTaskId["task-1"]).toBeUndefined();
  });

  it("does not reuse task PRs left behind after a workspace switch", async () => {
    listTaskPRsMock.mockResolvedValue({ task_prs: { "task-1": [] } });
    const { result } = renderHook(() => useHydrationWithStore("task-1"), {
      wrapper: createStateWrapper({ taskPRs: { byTaskId: { "task-1": [makePR()] } } }),
    });

    act(() => {
      result.current.store.getState().resetKanbanWorkspaceContext();
      result.current.store.getState().setActiveWorkspace("workspace-b");
    });
    await act(async () => {
      await result.current.hydration.hydrate();
    });

    expect(listTaskPRsMock).toHaveBeenCalledWith(["task-1"], { cache: "no-store" });
  });

  it("does not reuse task PRs after the workspace context generation changes", async () => {
    listTaskPRsMock.mockResolvedValue({ task_prs: { "task-1": [] } });
    const { result } = renderHook(() => useHydrationWithStore("task-1"), {
      wrapper: createStateWrapper({ taskPRs: { byTaskId: { "task-1": [makePR()] } } }),
    });

    act(() => {
      result.current.store.getState().resetKanbanWorkspaceContext();
    });
    await act(async () => {
      await result.current.hydration.hydrate();
    });

    expect(listTaskPRsMock).toHaveBeenCalledWith(["task-1"], { cache: "no-store" });
  });
});

describe("useTaskPRTooltipHydration retry states", () => {
  it("shows unavailable for an empty response and retries the next disclosure", async () => {
    const pr = makePR();
    listTaskPRsMock
      .mockResolvedValueOnce({ task_prs: { "task-1": [] } })
      .mockResolvedValueOnce({ task_prs: { "task-1": [pr] } });
    const { result } = renderHook(() => useHydrationWithStore("task-1"), {
      wrapper: createStateWrapper(),
    });

    await act(async () => {
      await result.current.hydration.hydrate();
    });
    expect(result.current.hydration.status).toBe("unavailable");

    await act(async () => {
      await result.current.hydration.hydrate();
    });

    expect(listTaskPRsMock).toHaveBeenCalledTimes(2);
    expect(result.current.store.getState().taskPRs.byTaskId["task-1"]).toEqual([pr]);
  });

  it("shows unavailable after a failed response and retries the next disclosure", async () => {
    const pr = makePR();
    listTaskPRsMock
      .mockRejectedValueOnce(new Error("request failed"))
      .mockResolvedValueOnce({ task_prs: { "task-1": [pr] } });
    const { result } = renderHook(() => useHydrationWithStore("task-1"), {
      wrapper: createStateWrapper(),
    });

    await act(async () => {
      await result.current.hydration.hydrate();
    });
    expect(result.current.hydration.status).toBe("unavailable");

    await act(async () => {
      await result.current.hydration.hydrate();
    });

    expect(listTaskPRsMock).toHaveBeenCalledTimes(2);
    expect(result.current.store.getState().taskPRs.byTaskId["task-1"]).toEqual([pr]);
  });

  it("does not resurrect an association deleted while the request was pending", async () => {
    const response = deferred<{ task_prs: Record<string, TaskPR[]> }>();
    listTaskPRsMock.mockReturnValue(response.promise);
    const { result } = renderHook(() => useHydrationWithStore("task-1"), {
      wrapper: createStateWrapper(),
    });
    const request = result.current.hydration.hydrate();

    act(() => {
      result.current.store.getState().removeTaskPR("task-1", "pr-1");
    });
    await act(async () => {
      response.resolve({ task_prs: { "task-1": [makePR()] } });
      await request;
    });

    expect(result.current.store.getState().taskPRs.byTaskId["task-1"]).toBeUndefined();
  });
});
