import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

const requestMock = vi.fn();
const listWorkspaceTaskPRsMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  listWorkspaceTaskPRs: listWorkspaceTaskPRsMock,
}));

import { useActiveTaskPR, useTaskPR, useWorkspacePRs } from "./use-task-pr";

const CREATED_AT = "2026-06-28T00:00:00Z";

function createQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

function wrapper({ children }: { children: ReactNode }) {
  const [queryClient] = useState(createQueryClient);
  return createElement(
    QueryClientProvider,
    { client: queryClient },
    createElement(StateProvider, null, children),
  );
}

function wrapperFor(queryClient: QueryClient, initialState?: Partial<AppState>) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(
      QueryClientProvider,
      { client: queryClient },
      createElement(StateProvider, { initialState, children }),
    );
  };
}

function taskPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "task-pr-1",
    task_id: "task-1",
    owner: "kdlbs",
    repo: "kandev",
    pr_number: 1130,
    pr_url: "https://github.com/kdlbs/kandev/pull/1130",
    pr_title: "Migrate to TanStack Query",
    head_branch: "feature/tanstack-migration",
    base_branch: "main",
    author_login: "developer",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "unknown",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: CREATED_AT,
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: CREATED_AT,
    ...overrides,
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  requestMock.mockReset();
  listWorkspaceTaskPRsMock.mockReset();
  listWorkspaceTaskPRsMock.mockResolvedValue({ task_prs: {} });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useTaskPR — permanent flag", () => {
  // The dominant production signal in the SyncWatchesBatched storm was the
  // frontend polling `github.task_pr.sync` every 5s for tasks whose repos
  // were deleted/inaccessible. The backend now returns `permanent: true`
  // on those responses; the hook must stop the retry interval cold.
  it("stops the 5s retry interval when the backend reports permanent: true", async () => {
    requestMock.mockResolvedValue({ prs: [], permanent: true });

    renderHook(() => useTaskPR("task-1"), { wrapper });

    // Initial freshness sync fires synchronously from the mount effect.
    // Flush the resolved promise so the permanent flag is applied before
    // the interval would otherwise fire.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(requestMock).toHaveBeenCalledTimes(1);

    // Advance well past several retry windows. If the permanent
    // short-circuit regressed, this would burst 5-6 additional calls
    // into requestMock.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000 * 6);
    });
    expect(requestMock).toHaveBeenCalledTimes(1);
  });

  // Without permanent, the existing retry cadence must still kick in so
  // tasks waiting on a freshly-pushed branch still get their PR detected.
  it("retries every 5s when permanent is absent and no PR is in the store", async () => {
    requestMock.mockResolvedValue({ prs: [] });

    renderHook(() => useTaskPR("task-1"), { wrapper });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(requestMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
    });
    expect(requestMock).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_000);
    });
    expect(requestMock).toHaveBeenCalledTimes(3);
  });
});

describe("useActiveTaskPR", () => {
  it("subscribes to cached active task PRs without starting PR sync", () => {
    const queryClient = createQueryClient();
    const wrapper = wrapperFor(queryClient, {
      tasks: { activeTaskId: "task-1" },
    } as Partial<AppState>);

    const { result } = renderHook(() => useActiveTaskPR(), { wrapper });

    expect(result.current).toBeNull();

    act(() => {
      queryClient.setQueryData(qk.integrations.github.taskPr("task-1"), [
        taskPR({ pr_number: 1512 }),
      ]);
    });

    expect(result.current?.pr_number).toBe(1512);
    expect(requestMock).not.toHaveBeenCalled();
  });
});

describe("useWorkspacePRs", () => {
  it("clears per-task PR caches that disappear from the workspace aggregate", async () => {
    vi.useRealTimers();
    const queryClient = createQueryClient();
    const taskOnePr = taskPR({ task_id: "task-1", pr_number: 1512 });
    const taskTwoPr = taskPR({ id: "task-pr-2", task_id: "task-2", pr_number: 1513 });
    listWorkspaceTaskPRsMock
      .mockResolvedValueOnce({
        task_prs: {
          "task-1": [taskOnePr],
          "task-2": [taskTwoPr],
        },
      })
      .mockResolvedValueOnce({
        task_prs: {
          "task-2": [taskTwoPr],
        },
      });

    const { result } = renderHook(() => useWorkspacePRs("workspace-1"), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current["task-1"]).toEqual([taskOnePr]));
    expect(queryClient.getQueryData(qk.integrations.github.taskPr("task-1"))).toEqual([taskOnePr]);

    await act(async () => {
      await queryClient.invalidateQueries({
        exact: true,
        queryKey: qk.integrations.github.prs("workspace-1"),
      });
    });

    await waitFor(() => expect(result.current["task-1"]).toBeUndefined());
    expect(queryClient.getQueryData(qk.integrations.github.taskPr("task-1"))).toEqual([]);
    expect(queryClient.getQueryData(qk.integrations.github.taskPr("task-2"))).toEqual([taskTwoPr]);
  });
});
