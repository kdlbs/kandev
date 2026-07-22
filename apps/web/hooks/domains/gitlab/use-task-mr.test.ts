import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { act, cleanup, render, renderHook, screen, waitFor } from "@testing-library/react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import type { GitLabStatus, TaskMR } from "@/lib/types/gitlab";

const fetchGitLabStatusMock = vi.fn<() => Promise<GitLabStatus | null>>();
const listWorkspaceTaskMRsMock =
  vi.fn<(workspaceId: string) => Promise<{ task_mrs: Record<string, TaskMR[]> } | null>>();
const EMPTY_TASK_MRS: Record<string, TaskMR[]> = {};
const WORKSPACE_MRS_TEST_ID = "workspace-mrs";

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  fetchGitLabStatus: () => fetchGitLabStatusMock(),
  listWorkspaceTaskMRs: (workspaceId: string) => listWorkspaceTaskMRsMock(workspaceId),
}));

import { useGitLabAvailable, useTaskMRs, useWorkspaceMRs } from "./use-task-mr";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, {
    initialState: { workspaces: { items: [], activeId: "ws-1" } },
    children,
  });
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

function WorkspaceMRProbe({ workspaceId }: { workspaceId: string }) {
  useWorkspaceMRs(workspaceId);
  const mrs = useAppStore((state) => state.taskMRs.byWorkspaceId[workspaceId] ?? EMPTY_TASK_MRS);
  return createElement("output", { "data-testid": WORKSPACE_MRS_TEST_ID }, JSON.stringify(mrs));
}

function workspaceProbe(workspaceId: string) {
  return createElement(StateProvider, {
    initialState: { workspaces: { items: [], activeId: workspaceId } },
    children: createElement(WorkspaceMRProbe, { key: workspaceId, workspaceId }),
  });
}

describe("useWorkspaceMRs", () => {
  beforeEach(() => {
    listWorkspaceTaskMRsMock.mockReset();
  });

  it("hydrates the store with the workspace's task MRs", async () => {
    const mr = makeMR({ task_id: "task-1" });
    listWorkspaceTaskMRsMock.mockResolvedValueOnce({ task_mrs: { "task-1": [mr] } });

    const { result } = renderHook(
      () => {
        useWorkspaceMRs("ws-1");
        return useAppStore((s) => s.taskMRs.byWorkspaceId["ws-1"] ?? EMPTY_TASK_MRS);
      },
      { wrapper },
    );

    await waitFor(() => expect(result.current["task-1"]).toEqual([mr]));
    expect(listWorkspaceTaskMRsMock).toHaveBeenCalledWith("ws-1");
  });

  it("does not refetch when the workspace id stays the same", async () => {
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

  it("clears MRs and invalidates in-flight requests when workspace becomes null", async () => {
    // First fetch is slow — we will switch to null before it resolves to
    // verify the in-flight result is dropped by the request-id guard.
    let resolveFirst: (v: { task_mrs: Record<string, TaskMR[]> }) => void = () => {};
    const firstPromise = new Promise<{ task_mrs: Record<string, TaskMR[]> }>((res) => {
      resolveFirst = res;
    });
    listWorkspaceTaskMRsMock.mockReturnValueOnce(firstPromise);

    const { result, rerender } = renderHook(
      ({ ws }: { ws: string | null }) => {
        useWorkspaceMRs(ws);
        return useAppStore((s) => s.taskMRs.byWorkspaceId["ws-1"] ?? EMPTY_TASK_MRS);
      },
      { wrapper, initialProps: { ws: "ws-1" as string | null } },
    );

    // Pre-populate the store so we can observe it being cleared.
    const setInitial = renderHook(() => useAppStore((s) => s.setTaskMRs), { wrapper });
    act(() => {
      setInitial.result.current("ws-1", { "task-1": [makeMR()] });
    });

    rerender({ ws: null });
    await waitFor(() => expect(result.current).toEqual({}));

    // The first fetch resolves *after* the null switch — its data must
    // NOT land in the store.
    const mr = makeMR({ task_id: "task-1" });
    await act(async () => {
      resolveFirst({ task_mrs: { "task-1": [mr] } });
    });
    await new Promise((r) => setTimeout(r, 10));
    expect(result.current).toEqual({});
  });

  it("clears workspace A immediately while workspace B fetch is deferred", async () => {
    const workspaceAMR = makeMR({ id: "a", task_id: "task-a", host: "https://a.gitlab.test" });
    let resolveWorkspaceB: (v: { task_mrs: Record<string, TaskMR[]> }) => void = () => {};
    const workspaceBPromise = new Promise<{ task_mrs: Record<string, TaskMR[]> }>((resolve) => {
      resolveWorkspaceB = resolve;
    });
    listWorkspaceTaskMRsMock
      .mockResolvedValueOnce({ task_mrs: { "task-a": [workspaceAMR] } })
      .mockReturnValueOnce(workspaceBPromise);

    const { result, rerender } = renderHook(
      ({ ws }: { ws: string }) => {
        useWorkspaceMRs(ws);
        return useAppStore((state) => state.taskMRs.byWorkspaceId[ws] ?? EMPTY_TASK_MRS);
      },
      { wrapper, initialProps: { ws: "ws-a" } },
    );
    await waitFor(() => expect(result.current["task-a"]).toEqual([workspaceAMR]));

    rerender({ ws: "ws-b" });
    await waitFor(() => expect(result.current).toEqual({}));

    const workspaceBMR = makeMR({ id: "b", task_id: "task-b", host: "https://b.gitlab.test" });
    await act(async () => {
      resolveWorkspaceB({ task_mrs: { "task-b": [workspaceBMR] } });
    });
    await waitFor(() => expect(result.current["task-b"]).toEqual([workspaceBMR]));
  });

  it("clears fetchedRef on failure so a workspace switch can retry that id later", async () => {
    listWorkspaceTaskMRsMock.mockRejectedValueOnce(new Error("boom"));
    const { rerender } = renderHook(({ ws }: { ws: string | null }) => useWorkspaceMRs(ws), {
      wrapper,
      initialProps: { ws: "ws-1" },
    });
    await waitFor(() => expect(listWorkspaceTaskMRsMock).toHaveBeenCalledTimes(1));

    // Bounce through another workspace and back to ws-1. Without the
    // failure-path reset of fetchedRef, the second visit to ws-1 would
    // be a no-op because the hook would still think the fetch succeeded.
    listWorkspaceTaskMRsMock.mockResolvedValueOnce({ task_mrs: {} });
    rerender({ ws: "ws-2" });
    await waitFor(() => expect(listWorkspaceTaskMRsMock).toHaveBeenCalledTimes(2));

    listWorkspaceTaskMRsMock.mockResolvedValueOnce({ task_mrs: {} });
    rerender({ ws: "ws-1" });
    await waitFor(() => expect(listWorkspaceTaskMRsMock).toHaveBeenCalledTimes(3));
  });
});

describe("useWorkspaceMRs cleanup", () => {
  it("ignores workspace A's deferred fetch after its loader unmounts and B loads", async () => {
    let resolveWorkspaceA: (v: { task_mrs: Record<string, TaskMR[]> }) => void = () => {};
    const workspaceAPromise = new Promise<{ task_mrs: Record<string, TaskMR[]> }>((resolve) => {
      resolveWorkspaceA = resolve;
    });
    const workspaceBMR = makeMR({ id: "b", task_id: "task-b" });
    listWorkspaceTaskMRsMock.mockReset();
    listWorkspaceTaskMRsMock
      .mockReturnValueOnce(workspaceAPromise)
      .mockResolvedValueOnce({ task_mrs: { "task-b": [workspaceBMR] } });

    const view = render(workspaceProbe("ws-a"));
    await waitFor(() => expect(listWorkspaceTaskMRsMock).toHaveBeenCalledWith("ws-a"));
    view.rerender(workspaceProbe("ws-b"));
    await waitFor(() =>
      expect(screen.getByTestId(WORKSPACE_MRS_TEST_ID).textContent).toContain('"id":"b"'),
    );

    await act(async () => {
      resolveWorkspaceA({ task_mrs: { "task-a": [makeMR({ id: "late-a" })] } });
    });

    expect(screen.getByTestId(WORKSPACE_MRS_TEST_ID).textContent).toContain('"id":"b"');
    expect(screen.getByTestId(WORKSPACE_MRS_TEST_ID).textContent).not.toContain("late-a");
  });
});

describe("useTaskMRs", () => {
  it("returns the same array reference across renders when empty", () => {
    const { result, rerender } = renderHook(() => useTaskMRs("task-empty"), { wrapper });
    const first = result.current;
    rerender();
    rerender();
    // If we returned `[]` literal each call, this would fail and the
    // zustand selector would loop forever.
    expect(result.current).toBe(first);
  });

  it("reads the task's MRs from the store", async () => {
    const mr = makeMR({ task_id: "task-1" });
    const { result } = renderHook(
      () => {
        const setTaskMRs = useAppStore((s) => s.setTaskMRs);
        const mrs = useTaskMRs("task-1");
        return { setTaskMRs, mrs };
      },
      { wrapper },
    );
    act(() => {
      result.current.setTaskMRs("ws-1", { "task-1": [mr] });
    });
    expect(result.current.mrs).toEqual([mr]);
  });

  it("hides workspace A's cached MRs on the first render for workspace B", () => {
    const workspaceAMR = makeMR({ task_id: "task-1" });
    const { result, rerender } = renderHook(
      ({ ws }: { ws: string }) => {
        const setTaskMRs = useAppStore((state) => state.setTaskMRs);
        const mrs = useTaskMRs("task-1", ws);
        return { setTaskMRs, mrs };
      },
      { wrapper, initialProps: { ws: "ws-a" } },
    );
    act(() => {
      result.current.setTaskMRs("ws-a", { "task-1": [workspaceAMR] });
    });
    expect(result.current.mrs).toEqual([workspaceAMR]);

    rerender({ ws: "ws-b" });

    expect(result.current.mrs).toEqual([]);
  });

  it("keeps workspace B's selected MRs stable after a delayed workspace A link", () => {
    const workspaceBMR = makeMR({ id: "b", task_id: "task-b" });
    const { result } = renderHook(
      () => {
        const setTaskMRs = useAppStore((state) => state.setTaskMRs);
        const setTaskMR = useAppStore((state) => state.setTaskMR);
        const mrs = useTaskMRs("task-b", "ws-b");
        return { setTaskMRs, setTaskMR, mrs };
      },
      { wrapper },
    );
    act(() => {
      result.current.setTaskMRs("ws-b", { "task-b": [workspaceBMR] });
    });
    expect(result.current.mrs).toEqual([workspaceBMR]);

    act(() => {
      result.current.setTaskMR("ws-a", "task-a", makeMR({ id: "late-a" }));
    });

    expect(result.current.mrs).toEqual([workspaceBMR]);
  });
});

describe("useGitLabAvailable", () => {
  beforeEach(() => {
    fetchGitLabStatusMock.mockReset();
  });

  it("returns true when GitLab is authenticated", async () => {
    fetchGitLabStatusMock.mockResolvedValue(makeStatus({ authenticated: true }));
    const { result } = renderHook(() => useGitLabAvailable(), { wrapper });
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("returns true when a token is configured but probe says unauthenticated", async () => {
    // token_configured is a softer signal — the integration is set up but
    // the probe might be stale. We want the integration to appear in menus.
    fetchGitLabStatusMock.mockResolvedValue(
      makeStatus({ authenticated: false, token_configured: true }),
    );
    const { result } = renderHook(() => useGitLabAvailable(), { wrapper });
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("returns false when neither flag is set", async () => {
    fetchGitLabStatusMock.mockResolvedValue(
      makeStatus({ authenticated: false, token_configured: false }),
    );
    const { result } = renderHook(() => useGitLabAvailable(), { wrapper });
    // The probe runs once on mount — give it a tick to land.
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("returns false when the probe rejects (offline / no client)", async () => {
    fetchGitLabStatusMock.mockRejectedValue(new Error("network down"));
    const { result } = renderHook(() => useGitLabAvailable(), { wrapper });
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("does not re-probe when the window regains focus", async () => {
    // Regression guard: previously the hook re-fetched on every focus event,
    // which hammered GET /api/v1/gitlab/status on every browser tab switch.
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
