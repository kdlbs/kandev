import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";

const mockListRepositories = vi.fn();
const mockSetRepositories = vi.fn();
const mockSetRepositoriesLoading = vi.fn();

type Repos = { id: string; name: string }[];
type MockState = {
  repositories: {
    itemsByWorkspaceId: Record<string, Repos>;
    loadingByWorkspaceId: Record<string, boolean>;
    loadedByWorkspaceId: Record<string, boolean>;
  };
  setRepositories: typeof mockSetRepositories;
  setRepositoriesLoading: typeof mockSetRepositoriesLoading;
};

let mockState: MockState;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/api", () => ({
  listRepositories: (...args: unknown[]) => mockListRepositories(...args),
}));

import { useRepositories } from "./use-repositories";

function setup(loaded: boolean) {
  vi.clearAllMocks();
  mockState = {
    repositories: {
      itemsByWorkspaceId: {},
      loadingByWorkspaceId: {},
      loadedByWorkspaceId: { "ws-1": loaded },
    },
    setRepositories: mockSetRepositories,
    setRepositoriesLoading: mockSetRepositoriesLoading,
  };
}

describe("useRepositories", () => {
  beforeEach(() => {
    mockListRepositories.mockResolvedValue({ repositories: [{ id: "r1", name: "Repo One" }] });
  });

  it("force-refreshes a fresh list even when the workspace is already loaded", async () => {
    setup(/* loaded */ true);
    renderHook(() => useRepositories("ws-1", true, true));
    await waitFor(() => expect(mockSetRepositories).toHaveBeenCalled());
    expect(mockListRepositories).toHaveBeenCalledWith("ws-1", undefined, { cache: "no-store" });
    expect(mockSetRepositories).toHaveBeenCalledWith("ws-1", [{ id: "r1", name: "Repo One" }]);
  });

  it("does not fetch on the lazy path when already loaded", async () => {
    setup(/* loaded */ true);
    renderHook(() => useRepositories("ws-1", true, false));
    await Promise.resolve();
    expect(mockListRepositories).not.toHaveBeenCalled();
  });

  it("fetches on the lazy path when not yet loaded", async () => {
    setup(/* loaded */ false);
    renderHook(() => useRepositories("ws-1", true, false));
    await waitFor(() => expect(mockSetRepositories).toHaveBeenCalled());
    expect(mockListRepositories).toHaveBeenCalledWith("ws-1", undefined, { cache: "no-store" });
  });

  it("does nothing when disabled or workspace is null", async () => {
    setup(/* loaded */ false);
    renderHook(() => useRepositories(null, true, true));
    renderHook(() => useRepositories("ws-1", false, true));
    await Promise.resolve();
    expect(mockListRepositories).not.toHaveBeenCalled();
  });

  it("refreshes an already-loaded workspace when requested", async () => {
    setup(/* loaded */ true);
    const { result } = renderHook(() => useRepositories("ws-1"));

    await result.current.refresh();

    expect(mockListRepositories).toHaveBeenCalledWith("ws-1", undefined, { cache: "no-store" });
    expect(mockSetRepositories).toHaveBeenCalledWith("ws-1", [{ id: "r1", name: "Repo One" }]);
  });
});
