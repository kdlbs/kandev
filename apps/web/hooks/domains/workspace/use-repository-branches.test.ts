import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useBranches, type BranchSource } from "./use-repository-branches";

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listBranches: vi.fn(),
  listRepositoryBranches: vi.fn(),
}));

import { listBranches, listRepositoryBranches } from "@/lib/api/domains/workspace-api";

const MOCK_BRANCHES = [
  { name: "main", type: "local" as const },
  { name: "dev", type: "local" as const },
];

const MOCK_RESPONSE = { branches: MOCK_BRANCHES, total: 2 };

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  }
  return { qc, Wrapper };
}

const ID_SOURCE: BranchSource = {
  kind: "id",
  workspaceId: "ws-1",
  repositoryId: "repo-1",
};

const PATH_SOURCE: BranchSource = {
  kind: "path",
  workspaceId: "ws-1",
  path: "/home/user/project",
};

describe("useBranches — id source", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (listBranches as ReturnType<typeof vi.fn>).mockResolvedValue(MOCK_RESPONSE);
    (listRepositoryBranches as ReturnType<typeof vi.fn>).mockResolvedValue(MOCK_RESPONSE);
  });

  it("fetches branches for an id-based source", async () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useBranches(ID_SOURCE), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.branches).toEqual(MOCK_BRANCHES);
    expect(listBranches).toHaveBeenCalledWith("ws-1", { repositoryId: "repo-1" });
  });

  it("returns empty branches when source is null", () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useBranches(null), { wrapper: Wrapper });
    expect(result.current.branches).toEqual([]);
    expect(result.current.refresh).toBeUndefined();
  });

  it("refresh calls listRepositoryBranches with refresh=true and updates cache", async () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useBranches(ID_SOURCE), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    const NEW_BRANCHES = [{ name: "feature", type: "local" as const }];
    (listRepositoryBranches as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      branches: NEW_BRANCHES,
      total: 1,
    });

    await act(async () => {
      await result.current.refresh?.();
    });

    expect(listRepositoryBranches).toHaveBeenCalledWith("repo-1", { refresh: true });
    // The refreshed data is written back to the cache; wait for re-render.
    await waitFor(() => {
      expect(result.current.branches).toEqual(NEW_BRANCHES);
    });
  });
});

describe("useBranches — path source", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (listBranches as ReturnType<typeof vi.fn>).mockResolvedValue(MOCK_RESPONSE);
  });

  it("fetches branches for a path-based source", async () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useBranches(PATH_SOURCE), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.branches).toEqual(MOCK_BRANCHES);
    expect(listBranches).toHaveBeenCalledWith("ws-1", { path: "/home/user/project" });
  });

  it("refresh for path source re-issues standard list call", async () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useBranches(PATH_SOURCE), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    await act(async () => {
      await result.current.refresh?.();
    });

    // listBranches called once for initial load + once for refresh
    expect(listBranches).toHaveBeenCalledTimes(2);
    expect(listBranches).toHaveBeenLastCalledWith("ws-1", { path: "/home/user/project" });
  });
});
