import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, waitFor } from "@testing-library/react";
import { renderHookWithQueryClient } from "@/test-utils/render-with-query";
import type { GitLabStatus, TaskMR, TaskMRsResponse } from "@/lib/types/gitlab";
import { qk } from "@/lib/query/keys";

// --- API mocks ---
const listWorkspaceTaskMRsMock = vi.fn<[string], Promise<TaskMRsResponse | null>>();
const fetchGitLabStatusMock = vi.fn<[], Promise<GitLabStatus | null>>();

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  listWorkspaceTaskMRs: (workspaceId: string) => listWorkspaceTaskMRsMock(workspaceId),
  fetchGitLabStatus: () => fetchGitLabStatusMock(),
}));

// useTaskMRs reads workspaces.activeId from Zustand — mock the store selector.
const activeIdMock = vi.fn<[], string | null>(() => "ws-1");
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: { workspaces: { activeId: string | null } }) => unknown) =>
    selector({ workspaces: { activeId: activeIdMock() } }),
}));

import {
  useWorkspaceMRs,
  useTaskMRs,
  useGitLabAvailable,
} from "@/hooks/domains/gitlab/use-task-mr";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

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

// ---------------------------------------------------------------------------
// useWorkspaceMRs
// ---------------------------------------------------------------------------
describe("useWorkspaceMRs", () => {
  beforeEach(() => {
    listWorkspaceTaskMRsMock.mockReset();
  });

  it("fetches MRs for the given workspace and populates the cache", async () => {
    const mr = makeMR({ task_id: "task-1" });
    listWorkspaceTaskMRsMock.mockResolvedValueOnce({ task_mrs: { "task-1": [mr] } });

    const { client } = renderHookWithQueryClient(() => useWorkspaceMRs("ws-1"));

    await waitFor(() =>
      expect(client.getQueryData(qk.gitlab.mrs("ws-1"))).toEqual({
        task_mrs: { "task-1": [mr] },
      }),
    );
    expect(listWorkspaceTaskMRsMock).toHaveBeenCalledWith("ws-1");
  });

  it("is disabled (no fetch) when workspaceId is null", () => {
    listWorkspaceTaskMRsMock.mockResolvedValue({ task_mrs: {} });
    renderHookWithQueryClient(() => useWorkspaceMRs(null));
    expect(listWorkspaceTaskMRsMock).not.toHaveBeenCalled();
  });

  it("deduplicates concurrent fetches for the same workspace", async () => {
    listWorkspaceTaskMRsMock.mockResolvedValue({ task_mrs: {} });
    const { client } = renderHookWithQueryClient(() => {
      useWorkspaceMRs("ws-1");
      useWorkspaceMRs("ws-1");
    });
    await waitFor(() => expect(client.getQueryData(qk.gitlab.mrs("ws-1"))).toBeDefined());
    expect(listWorkspaceTaskMRsMock).toHaveBeenCalledTimes(1);
  });
});

// ---------------------------------------------------------------------------
// useTaskMRs
// ---------------------------------------------------------------------------
describe("useTaskMRs", () => {
  beforeEach(() => {
    listWorkspaceTaskMRsMock.mockReset();
    activeIdMock.mockReturnValue("ws-1");
  });

  it("returns the task's MRs from the cache", async () => {
    const mr = makeMR({ task_id: "task-1" });
    listWorkspaceTaskMRsMock.mockResolvedValueOnce({ task_mrs: { "task-1": [mr] } });

    const { result } = renderHookWithQueryClient(() => useTaskMRs("task-1"));

    await waitFor(() => expect(result.current).toEqual([mr]));
  });

  it("returns a stable empty array for unknown task ids", async () => {
    listWorkspaceTaskMRsMock.mockResolvedValue({ task_mrs: {} });
    const { result } = renderHookWithQueryClient(() => useTaskMRs("task-unknown"));
    const first = result.current;
    await waitFor(() => expect(listWorkspaceTaskMRsMock).toHaveBeenCalled());
    expect(result.current).toBe(first); // referentially stable
  });

  it("returns empty array when taskId is null", async () => {
    listWorkspaceTaskMRsMock.mockResolvedValue({ task_mrs: {} });
    const { result } = renderHookWithQueryClient(() => useTaskMRs(null));
    expect(result.current).toEqual([]);
  });

  it("returns empty array when no active workspace", async () => {
    activeIdMock.mockReturnValue(null);
    const { result } = renderHookWithQueryClient(() => useTaskMRs("task-1"));
    expect(result.current).toEqual([]);
    expect(listWorkspaceTaskMRsMock).not.toHaveBeenCalled();
  });

  it("pre-seeded cache is returned synchronously without a fetch", async () => {
    const mr = makeMR({ task_id: "task-seeded" });
    listWorkspaceTaskMRsMock.mockResolvedValue({ task_mrs: { "task-seeded": [mr] } });

    // Seed the cache before rendering the hook so the data is available.
    const client = renderHookWithQueryClient(() => null).client;
    client.setQueryData(qk.gitlab.mrs("ws-1"), { task_mrs: { "task-seeded": [mr] } });

    const { result } = renderHookWithQueryClient(() => useTaskMRs("task-seeded"), { client });
    await waitFor(() => expect(result.current).toEqual([mr]));
    // Pre-seeded data is within staleTime — no network request should be made.
    expect(listWorkspaceTaskMRsMock).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// useGitLabAvailable
// ---------------------------------------------------------------------------
describe("useGitLabAvailable", () => {
  beforeEach(() => {
    fetchGitLabStatusMock.mockReset();
  });

  it("returns true when GitLab is authenticated", async () => {
    fetchGitLabStatusMock.mockResolvedValue(makeStatus({ authenticated: true }));
    const { result } = renderHookWithQueryClient(() => useGitLabAvailable());
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("returns true when a token is configured but unauthenticated", async () => {
    fetchGitLabStatusMock.mockResolvedValue(
      makeStatus({ authenticated: false, token_configured: true }),
    );
    const { result } = renderHookWithQueryClient(() => useGitLabAvailable());
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("returns false when neither flag is set", async () => {
    fetchGitLabStatusMock.mockResolvedValue(
      makeStatus({ authenticated: false, token_configured: false }),
    );
    const { result } = renderHookWithQueryClient(() => useGitLabAvailable());
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("returns false when the probe rejects", async () => {
    fetchGitLabStatusMock.mockRejectedValue(new Error("network down"));
    const { result } = renderHookWithQueryClient(() => useGitLabAvailable());
    await waitFor(() => expect(fetchGitLabStatusMock).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });
});
