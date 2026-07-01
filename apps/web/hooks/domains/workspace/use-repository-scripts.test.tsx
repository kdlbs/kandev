import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { qk } from "@/lib/query/keys";
import { repositoryId as toRepositoryId, type RepositoryScript } from "@/lib/types/http";
import { useRepositoryScripts } from "./use-repository-scripts";

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

describe("useRepositoryScripts", () => {
  afterEach(() => {
    cleanup();
  });

  it("reads repository scripts from TanStack Query without a Zustand store", () => {
    const queryClient = createQueryClient();
    const scripts: RepositoryScript[] = [
      {
        id: "script-1",
        repository_id: toRepositoryId("repo-1"),
        name: "Setup",
        command: "pnpm install",
        position: 0,
        created_at: "2026-06-24T00:00:00Z",
        updated_at: "2026-06-24T00:00:00Z",
      },
    ];
    queryClient.setQueryData(qk.workspaces.repositoryScripts("repo-1"), scripts);

    const { result } = renderHook(() => useRepositoryScripts("repo-1"), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current).toMatchObject({
      scripts,
      isLoaded: true,
    });
  });
});
