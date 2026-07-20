import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listBranches, listRepositoryBranches } from "@/lib/api/domains/workspace-api";
import { qk } from "@/lib/query/keys";
import type { Branch } from "@/lib/types/http";
import { useBranches, type BranchSource } from "./use-repository-branches";

const WORKSPACE_ID = "workspace-1";
const REPO_ID = "repo-1";
const LOCAL_PATH = "/repo";

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listBranches: vi.fn(),
  listRepositoryBranches: vi.fn(),
}));

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: Infinity,
      },
    },
  });
}

function wrapperFor(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

describe("useBranches", () => {
  beforeEach(() => {
    vi.mocked(listBranches).mockReset();
    vi.mocked(listRepositoryBranches).mockReset();
  });

  afterEach(() => {
    cleanup();
  });

  it("reads repository branches from the workspace Query cache without a Zustand store", () => {
    const queryClient = createQueryClient();
    const branches: Branch[] = [{ name: "main", type: "local" }];
    const source: BranchSource = {
      kind: "id",
      workspaceId: WORKSPACE_ID,
      repositoryId: REPO_ID,
    };
    queryClient.setQueryData(
      qk.workspaces.branches(WORKSPACE_ID, { repositoryId: REPO_ID }),
      branches,
    );

    const { result } = renderHook(() => useBranches(source), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current).toMatchObject({
      branches,
      isLoading: false,
    });
  });

  it("reads path-based branches from the workspace Query cache without a Zustand store", () => {
    const queryClient = createQueryClient();
    const branches: Branch[] = [{ name: "develop", type: "local" }];
    const source: BranchSource = {
      kind: "path",
      workspaceId: WORKSPACE_ID,
      path: LOCAL_PATH,
    };
    queryClient.setQueryData(qk.workspaces.branches(WORKSPACE_ID, { path: LOCAL_PATH }), branches);

    const { result } = renderHook(() => useBranches(source), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current).toMatchObject({
      branches,
      isLoading: false,
    });
  });

  it("cold-loads id-based rows through the workspace branch endpoint", async () => {
    vi.mocked(listBranches).mockResolvedValue({
      branches: [{ name: "remote-main", type: "remote" }],
      total: 1,
    });
    const queryClient = createQueryClient();
    const source: BranchSource = {
      kind: "id",
      workspaceId: WORKSPACE_ID,
      repositoryId: REPO_ID,
    };

    const { result } = renderHook(() => useBranches(source), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.branches).toHaveLength(1));

    expect(listBranches).toHaveBeenCalledWith(
      WORKSPACE_ID,
      { repositoryId: REPO_ID },
      expect.any(Object),
    );
    expect(listRepositoryBranches).not.toHaveBeenCalled();
  });

  it("manual refresh forces the repository refresh endpoint and updates the active workspace cache", async () => {
    const initialBranches: Branch[] = [{ name: "main", type: "local" }];
    const refreshedBranches: Branch[] = [{ name: "feature/refreshed", type: "local" }];
    vi.mocked(listRepositoryBranches).mockResolvedValue({
      branches: refreshedBranches,
      total: 1,
    });
    const queryClient = createQueryClient();
    const source: BranchSource = {
      kind: "id",
      workspaceId: WORKSPACE_ID,
      repositoryId: REPO_ID,
    };
    queryClient.setQueryData(
      qk.workspaces.branches(WORKSPACE_ID, { repositoryId: REPO_ID }),
      initialBranches,
    );

    const { result } = renderHook(() => useBranches(source), {
      wrapper: wrapperFor(queryClient),
    });

    await act(async () => {
      await result.current.refresh?.();
    });

    expect(listRepositoryBranches).toHaveBeenCalledWith(
      REPO_ID,
      { refresh: true },
      expect.any(Object),
    );
    expect(
      queryClient.getQueryData(qk.workspaces.branches(WORKSPACE_ID, { repositoryId: REPO_ID })),
    ).toEqual(refreshedBranches);
  });
});
