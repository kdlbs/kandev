import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { qk } from "@/lib/query/keys";
import type { GitLabStatus, TaskMR } from "@/lib/types/gitlab";

const fetchGitLabStatusMock = vi.fn<() => Promise<GitLabStatus | null>>();
const listWorkspaceTaskMRsMock =
  vi.fn<(workspaceId: string) => Promise<{ task_mrs: Record<string, TaskMR[]> } | null>>();

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  fetchGitLabStatus: () => fetchGitLabStatusMock(),
  listWorkspaceTaskMRs: (workspaceId: string) => listWorkspaceTaskMRsMock(workspaceId),
}));

import { useGitLabAvailable, useTaskMRs, useWorkspaceMRs } from "./use-task-mr";

function createQueryHarness() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  });
  function wrapper({ children }: { children: ReactNode }) {
    const [client] = useState(queryClient);
    return createElement(QueryClientProvider, { client }, children);
  }
  return { queryClient, wrapper };
}

afterEach(() => cleanup());

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "mr-1",
    task_id: "task-1",
    host: "https://gitlab.com",
    project_path: "acme/api",
    mr_iid: 1,
    mr_url: "",
    mr_title: "Test",
    head_branch: "feat",
    base_branch: "main",
    author_username: "alice",
    state: "open",
    approval_state: "",
    pipeline_state: "",
    merge_status: "",
    draft: false,
    approval_count: 0,
    required_approvals: 0,
    pipeline_jobs_total: 0,
    pipeline_jobs_pass: 0,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function makeStatus(overrides: Partial<GitLabStatus> = {}): GitLabStatus {
  return {
    authenticated: true,
    username: "alice",
    auth_method: "pat",
    host: "https://gitlab.com",
    token_configured: true,
    required_scopes: ["api"],
    ...overrides,
  };
}

describe("useWorkspaceMRs", () => {
  beforeEach(() => {
    listWorkspaceTaskMRsMock.mockReset();
  });

  it("returns the workspace task MRs and seeds per-task query caches", async () => {
    const { wrapper } = createQueryHarness();
    const mr = makeMR({ task_id: "task-1" });
    listWorkspaceTaskMRsMock.mockResolvedValueOnce({ task_mrs: { "task-1": [mr] } });

    const { result } = renderHook(
      () => {
        const mrsByTask = useWorkspaceMRs("ws-1");
        const taskMrs = useTaskMRs("task-1");
        return { mrsByTask, taskMrs };
      },
      { wrapper },
    );

    await waitFor(() => expect(result.current.mrsByTask["task-1"]).toEqual([mr]));
    await waitFor(() => expect(result.current.taskMrs).toEqual([mr]));
    expect(listWorkspaceTaskMRsMock).toHaveBeenCalledWith("ws-1");
  });

  it("clears per-task caches for MRs missing from the latest workspace result", async () => {
    const { queryClient, wrapper } = createQueryHarness();
    const mr = makeMR({ task_id: "task-1" });
    listWorkspaceTaskMRsMock.mockResolvedValueOnce({ task_mrs: { "task-1": [mr] } });

    const { result } = renderHook(
      () => {
        const mrsByTask = useWorkspaceMRs("ws-1");
        const taskMrs = useTaskMRs("task-1");
        return { mrsByTask, taskMrs };
      },
      { wrapper },
    );

    await waitFor(() => expect(result.current.taskMrs).toEqual([mr]));

    act(() => {
      queryClient.setQueryData(qk.integrations.gitlab.mrs("ws-1"), { task_mrs: {} });
    });

    await waitFor(() => expect(result.current.mrsByTask).toEqual({}));
    await waitFor(() => expect(result.current.taskMrs).toEqual([]));
    expect(queryClient.getQueryData(qk.integrations.gitlab.taskMr("task-1"))).toEqual([]);
  });

  it("does not refetch when the workspace id stays the same", async () => {
    const { wrapper } = createQueryHarness();
    listWorkspaceTaskMRsMock.mockResolvedValue({ task_mrs: {} });
    const { rerender } = renderHook(({ ws }: { ws: string | null }) => useWorkspaceMRs(ws), {
      wrapper,
      initialProps: { ws: "ws-1" },
    });

    await waitFor(() => expect(listWorkspaceTaskMRsMock).toHaveBeenCalledTimes(1));
    rerender({ ws: "ws-1" });
    rerender({ ws: "ws-1" });
    expect(listWorkspaceTaskMRsMock).toHaveBeenCalledTimes(1);
  });

  it("does not expose stale MRs when workspace becomes null", async () => {
    const { wrapper } = createQueryHarness();
    let resolveFirst: (v: { task_mrs: Record<string, TaskMR[]> }) => void = () => {};
    const firstPromise = new Promise<{ task_mrs: Record<string, TaskMR[]> }>((res) => {
      resolveFirst = res;
    });
    listWorkspaceTaskMRsMock.mockReturnValueOnce(firstPromise);

    const { result, rerender } = renderHook(
      ({ ws }: { ws: string | null }) => useWorkspaceMRs(ws),
      { wrapper, initialProps: { ws: "ws-1" as string | null } },
    );

    rerender({ ws: null });
    expect(result.current).toEqual({});

    await act(async () => {
      resolveFirst({ task_mrs: { "task-1": [makeMR({ task_id: "task-1" })] } });
    });
    await new Promise((r) => setTimeout(r, 10));
    expect(result.current).toEqual({});
  });

  it("fetches again when switching away from and back to a workspace after failure", async () => {
    const { wrapper } = createQueryHarness();
    listWorkspaceTaskMRsMock.mockRejectedValueOnce(new Error("boom"));
    const { rerender } = renderHook(({ ws }: { ws: string | null }) => useWorkspaceMRs(ws), {
      wrapper,
      initialProps: { ws: "ws-1" },
    });
    await waitFor(() => expect(listWorkspaceTaskMRsMock).toHaveBeenCalledTimes(1));

    listWorkspaceTaskMRsMock.mockResolvedValueOnce({ task_mrs: {} });
    rerender({ ws: "ws-2" });
    await waitFor(() => expect(listWorkspaceTaskMRsMock).toHaveBeenCalledTimes(2));

    listWorkspaceTaskMRsMock.mockResolvedValueOnce({ task_mrs: {} });
    rerender({ ws: "ws-1" });
    await waitFor(() => expect(listWorkspaceTaskMRsMock).toHaveBeenCalledTimes(3));
  });
});

describe("useTaskMRs", () => {
  it("returns the same array reference across renders when empty", () => {
    const { wrapper } = createQueryHarness();
    const { result, rerender } = renderHook(() => useTaskMRs("task-empty"), { wrapper });
    const first = result.current;
    rerender();
    rerender();
    expect(result.current).toBe(first);
  });

  it("reads the task's MRs from the query cache", async () => {
    const { queryClient, wrapper } = createQueryHarness();
    const mr = makeMR({ task_id: "task-1" });

    const { result } = renderHook(() => useTaskMRs("task-1"), { wrapper });
    act(() => {
      queryClient.setQueryData(qk.integrations.gitlab.taskMr("task-1"), [mr]);
    });

    await waitFor(() => expect(result.current).toEqual([mr]));
  });
});

describe("useGitLabAvailable", () => {
  beforeEach(() => {
    fetchGitLabStatusMock.mockReset();
  });

  it("returns true when GitLab is authenticated", async () => {
    const { wrapper } = createQueryHarness();
    fetchGitLabStatusMock.mockResolvedValue(makeStatus({ authenticated: true }));
    const { result } = renderHook(() => useGitLabAvailable(), { wrapper });
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("returns true when a token is configured but probe says unauthenticated", async () => {
    const { wrapper } = createQueryHarness();
    fetchGitLabStatusMock.mockResolvedValue(
      makeStatus({ authenticated: false, token_configured: true }),
    );
    const { result } = renderHook(() => useGitLabAvailable(), { wrapper });
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("returns false when neither flag is set", async () => {
    const { wrapper } = createQueryHarness();
    fetchGitLabStatusMock.mockResolvedValue(
      makeStatus({ authenticated: false, token_configured: false }),
    );
    const { result } = renderHook(() => useGitLabAvailable(), { wrapper });
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("returns false when the probe rejects (offline / no client)", async () => {
    const { wrapper } = createQueryHarness();
    fetchGitLabStatusMock.mockRejectedValue(new Error("network down"));
    const { result } = renderHook(() => useGitLabAvailable(), { wrapper });
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("does not re-probe when the window regains focus", async () => {
    const { wrapper } = createQueryHarness();
    fetchGitLabStatusMock.mockResolvedValue(makeStatus());
    renderHook(() => useGitLabAvailable(), { wrapper });
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalledTimes(1));
    act(() => {
      window.dispatchEvent(new Event("focus"));
      window.dispatchEvent(new Event("focus"));
    });
    await new Promise((r) => setTimeout(r, 10));
    expect(fetchGitLabStatusMock).toHaveBeenCalledTimes(1);
  });
});
