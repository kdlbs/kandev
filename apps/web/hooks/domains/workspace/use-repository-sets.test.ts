import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

import type { RepositorySet } from "@/lib/types/http";
import { repositoryId, workspaceId } from "@/lib/types/ids";

const mockListRepositorySets = vi.fn();
const mockSetRepositorySets = vi.fn();
const mockSetRepositorySetsLoading = vi.fn();

type MockState = {
  repositorySets: {
    itemsByWorkspaceId: Record<string, RepositorySet[]>;
    loadingByWorkspaceId: Record<string, boolean>;
    loadedByWorkspaceId: Record<string, boolean>;
    revisionByWorkspaceId: Record<string, number>;
  };
  setRepositorySets: typeof mockSetRepositorySets;
  setRepositorySetsLoading: typeof mockSetRepositorySetsLoading;
};

let mockState: MockState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/api", () => ({
  listRepositorySets: (...args: unknown[]) => mockListRepositorySets(...args),
}));

import { useRepositorySets } from "./use-repository-sets";

function repositorySet(id: string): RepositorySet {
  return {
    id,
    workspace_id: workspaceId("ws-1"),
    name: id,
    description: "",
    repositories: [{ repository_id: repositoryId("repo-web"), position: 0 }],
    created_at: "2026-08-17T09:00:00Z",
    updated_at: "2026-08-17T09:00:00Z",
  };
}

function setup(options: { loaded: boolean; items?: RepositorySet[] }) {
  mockState = {
    repositorySets: {
      itemsByWorkspaceId: options.items ? { "ws-1": options.items } : {},
      loadingByWorkspaceId: {},
      loadedByWorkspaceId: { "ws-1": options.loaded },
      revisionByWorkspaceId: {},
    },
    setRepositorySets: mockSetRepositorySets,
    setRepositorySetsLoading: mockSetRepositorySetsLoading,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mockListRepositorySets.mockResolvedValue({ repository_sets: [repositorySet("set-1")], total: 1 });
});

describe("useRepositorySets", () => {
  it("fetches once when the workspace is not yet loaded", async () => {
    setup({ loaded: false });

    renderHook(() => useRepositorySets("ws-1"));

    await waitFor(() => expect(mockSetRepositorySets).toHaveBeenCalled());
    expect(mockListRepositorySets).toHaveBeenCalledTimes(1);
    // The third argument is the revision read before the request; the slice drops
    // the response when a WebSocket event moved it meanwhile.
    expect(mockSetRepositorySets).toHaveBeenCalledWith("ws-1", [repositorySet("set-1")], 0);
  });

  it("does not fetch when boot already hydrated the workspace, even with no sets", async () => {
    // Boot marks an empty workspace loaded; refetching on every dialog open would
    // undo the point of hydrating it.
    setup({ loaded: true, items: [] });

    const { result } = renderHook(() => useRepositorySets("ws-1"));

    expect(mockListRepositorySets).not.toHaveBeenCalled();
    expect(result.current.sets).toEqual([]);
  });

  it("returns the hydrated sets from the store", () => {
    setup({ loaded: true, items: [repositorySet("set-1"), repositorySet("set-2")] });

    const { result } = renderHook(() => useRepositorySets("ws-1"));

    expect(result.current.sets).toHaveLength(2);
  });

  it("does nothing without a workspace id", () => {
    setup({ loaded: false });

    const { result } = renderHook(() => useRepositorySets(null));

    expect(mockListRepositorySets).not.toHaveBeenCalled();
    expect(result.current.sets).toEqual([]);
  });

  it("does nothing when disabled", () => {
    setup({ loaded: false });

    renderHook(() => useRepositorySets("ws-1", false));

    expect(mockListRepositorySets).not.toHaveBeenCalled();
  });

  it("leaves the workspace unloaded after a failed fetch so the next mount retries", async () => {
    setup({ loaded: false });
    mockListRepositorySets.mockRejectedValue(new Error("offline"));

    renderHook(() => useRepositorySets("ws-1"));

    await waitFor(() => expect(mockSetRepositorySetsLoading).toHaveBeenCalledWith("ws-1", false));
    expect(mockSetRepositorySets).not.toHaveBeenCalled();
  });

  it("refresh re-reads the list and keeps the cache on failure", async () => {
    setup({ loaded: true, items: [repositorySet("set-1")] });

    const { result } = renderHook(() => useRepositorySets("ws-1"));
    await result.current.refresh();

    expect(mockListRepositorySets).toHaveBeenCalledTimes(1);
    expect(mockSetRepositorySets).toHaveBeenCalledWith("ws-1", [repositorySet("set-1")], 0);

    mockListRepositorySets.mockRejectedValue(new Error("offline"));
    mockSetRepositorySets.mockClear();
    await result.current.refresh();

    expect(mockSetRepositorySets).not.toHaveBeenCalled();
  });
});
