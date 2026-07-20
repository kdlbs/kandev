import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

const listBranchesMock = vi.fn();
const setRepositoryBranchesMock = vi.fn();
const setRepositoryBranchesLoadingMock = vi.fn();

const mockState = {
  repositoryBranches: {
    itemsByRepositoryId: {} as Record<string, unknown[]>,
    loadedByRepositoryId: {} as Record<string, boolean>,
    loadingByRepositoryId: {} as Record<string, boolean>,
  },
  setRepositoryBranches: setRepositoryBranchesMock,
  setRepositoryBranchesLoading: setRepositoryBranchesLoadingMock,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mockState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listBranches: (...args: unknown[]) => listBranchesMock(...args),
}));

import { useBranches, type BranchSource } from "./use-repository-branches";

const WORKSPACE_ID = "workspace-1";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useBranches", () => {
  it("loads a new repository while the prior repository request is still pending", async () => {
    listBranchesMock
      .mockImplementationOnce(() => new Promise(() => undefined))
      .mockResolvedValueOnce({ branches: [{ name: "main", type: "remote" }] });

    const sourceA: BranchSource = {
      kind: "id",
      workspaceId: WORKSPACE_ID,
      repositoryId: "repo-a",
    };
    const sourceB: BranchSource = {
      kind: "id",
      workspaceId: WORKSPACE_ID,
      repositoryId: "repo-b",
    };
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: queryClient }, children);
    const { rerender } = renderHook(({ source }: { source: BranchSource }) => useBranches(source), {
      initialProps: { source: sourceA },
      wrapper,
    });

    await waitFor(() =>
      expect(listBranchesMock).toHaveBeenCalledWith(
        WORKSPACE_ID,
        { repositoryId: "repo-a" },
        expect.any(Object),
      ),
    );
    rerender({ source: sourceB });

    await waitFor(() =>
      expect(listBranchesMock).toHaveBeenCalledWith(
        WORKSPACE_ID,
        { repositoryId: "repo-b" },
        expect.any(Object),
      ),
    );
  });
});
