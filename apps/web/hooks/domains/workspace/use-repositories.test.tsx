import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { qk } from "@/lib/query/keys";
import type { Repository } from "@/lib/types/http";
import { useRepositories } from "./use-repositories";
import { useRepository } from "./use-repository";

const WORKSPACE_ID = "workspace-1";
const REPO_ID = "repo-1";
const OTHER_REPO_ID = "repo-2";

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listRepositories: vi.fn(),
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

function repo(id: string, name: string): Repository {
  return {
    id,
    workspace_id: WORKSPACE_ID,
    name,
    local_path: `/workspace/${name}`,
  } as Repository;
}

describe("useRepositories", () => {
  beforeEach(() => {
    vi.mocked(listRepositories).mockReset();
  });

  afterEach(() => {
    cleanup();
  });

  it("reads workspace repositories from the Query cache without a Zustand store", () => {
    const queryClient = createQueryClient();
    const repositories = [repo(REPO_ID, "frontend")];
    queryClient.setQueryData(qk.workspaces.repositories(WORKSPACE_ID), repositories);

    const { result } = renderHook(() => useRepositories(WORKSPACE_ID), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current).toMatchObject({
      repositories,
      isLoading: false,
    });
  });

  it("cold-loads workspace repositories through the query option", async () => {
    const repositories = [repo(REPO_ID, "frontend")];
    vi.mocked(listRepositories).mockResolvedValue({ repositories, total: 1 });
    const queryClient = createQueryClient();

    const { result } = renderHook(() => useRepositories(WORKSPACE_ID), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.repositories).toEqual(repositories));

    expect(listRepositories).toHaveBeenCalledWith(WORKSPACE_ID, undefined, expect.any(Object));
    expect(queryClient.getQueryData(qk.workspaces.repositories(WORKSPACE_ID))).toEqual(
      repositories,
    );
  });

  it("force-refreshes a fresh list even when cached repositories exist", async () => {
    const cached = [repo(REPO_ID, "cached")];
    const fresh = [repo(OTHER_REPO_ID, "fresh")];
    vi.mocked(listRepositories).mockResolvedValue({ repositories: fresh, total: 1 });
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workspaces.repositories(WORKSPACE_ID), cached);

    const { result } = renderHook(() => useRepositories(WORKSPACE_ID, true, true), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.repositories).toEqual(cached);
    await waitFor(() => expect(result.current.repositories).toEqual(fresh));

    expect(listRepositories).toHaveBeenCalledWith(WORKSPACE_ID, undefined, expect.any(Object));
  });

  it("does not retry a failed forced refresh on rerender", async () => {
    const cached = [repo(REPO_ID, "cached")];
    vi.mocked(listRepositories).mockRejectedValueOnce(new Error("temporary failure"));
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workspaces.repositories(WORKSPACE_ID), cached);

    const { result, rerender } = renderHook(() => useRepositories(WORKSPACE_ID, true, true), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.repositories).toEqual(cached);
    await waitFor(() => expect(listRepositories).toHaveBeenCalledTimes(1));

    rerender();
    await Promise.resolve();

    expect(listRepositories).toHaveBeenCalledTimes(1);
    expect(result.current.repositories).toEqual(cached);
  });

  it("does not fetch when disabled or workspace is missing", async () => {
    const queryClient = createQueryClient();

    renderHook(() => useRepositories(null, true, true), {
      wrapper: wrapperFor(queryClient),
    });
    renderHook(() => useRepositories(WORKSPACE_ID, false, true), {
      wrapper: wrapperFor(queryClient),
    });

    await Promise.resolve();

    expect(listRepositories).not.toHaveBeenCalled();
  });
});

describe("useRepository", () => {
  afterEach(() => {
    cleanup();
  });

  it("finds a repository across workspace Query caches without a Zustand store", () => {
    const queryClient = createQueryClient();
    const target = repo(REPO_ID, "frontend");
    queryClient.setQueryData(qk.workspaces.repositories(WORKSPACE_ID), [
      repo(OTHER_REPO_ID, "backend"),
      target,
    ]);

    const { result } = renderHook(() => useRepository(REPO_ID), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current).toEqual(target);
  });
});
