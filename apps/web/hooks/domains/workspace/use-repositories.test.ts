import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useRepositories } from "./use-repositories";

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listRepositories: vi.fn(),
}));

import { listRepositories } from "@/lib/api/domains/workspace-api";

const MOCK_REPOS = [
  {
    id: "repo-1",
    workspace_id: "ws-1",
    name: "my-repo",
    source_type: "github",
    local_path: "/tmp/repo",
    provider: "github",
    provider_repo_id: "123",
    provider_owner: "acme",
    provider_name: "my-repo",
    default_branch: "main",
    worktree_branch_prefix: "kandev/",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
];

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  }
  return { qc, Wrapper };
}

describe("useRepositories", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (listRepositories as ReturnType<typeof vi.fn>).mockResolvedValue({
      repositories: MOCK_REPOS,
      total: 1,
    });
  });

  it("returns empty array while loading", () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useRepositories("ws-1"), { wrapper: Wrapper });
    expect(result.current.repositories).toEqual([]);
    expect(result.current.isLoading).toBe(true);
  });

  it("fetches and returns repositories", async () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useRepositories("ws-1"), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.repositories).toEqual(MOCK_REPOS);
    expect(listRepositories).toHaveBeenCalledWith("ws-1");
  });

  it("returns empty array when workspaceId is null", () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useRepositories(null), { wrapper: Wrapper });
    expect(result.current.repositories).toEqual([]);
    expect(result.current.isLoading).toBe(false);
    expect(listRepositories).not.toHaveBeenCalled();
  });

  it("does not fetch when enabled=false", () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useRepositories("ws-1", false), { wrapper: Wrapper });
    expect(result.current.repositories).toEqual([]);
    expect(listRepositories).not.toHaveBeenCalled();
  });
});
