import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useWorkspaces, useWorkspace } from "./use-workspaces";

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listWorkspaces: vi.fn(),
  listRepositories: vi.fn(),
  listBranches: vi.fn(),
  listRepositoryScripts: vi.fn(),
}));

import { listWorkspaces } from "@/lib/api/domains/workspace-api";

const TS = "2026-01-01T00:00:00Z";

const MOCK_WORKSPACES = [
  {
    id: "ws-1",
    name: "Alpha",
    description: null,
    owner_id: "user-1",
    created_at: TS,
    updated_at: TS,
  },
  {
    id: "ws-2",
    name: "Beta",
    description: null,
    owner_id: "user-1",
    created_at: TS,
    updated_at: TS,
  },
];

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  }
  return { qc, Wrapper };
}

describe("useWorkspaces", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (listWorkspaces as ReturnType<typeof vi.fn>).mockResolvedValue({
      workspaces: MOCK_WORKSPACES,
      total: 2,
    });
  });

  it("returns empty array while loading", () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useWorkspaces(), { wrapper: Wrapper });
    expect(result.current.workspaces).toEqual([]);
    expect(result.current.isLoading).toBe(true);
  });

  it("fetches and returns the workspaces list", async () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useWorkspaces(), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.workspaces).toEqual(MOCK_WORKSPACES);
  });
});

describe("useWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (listWorkspaces as ReturnType<typeof vi.fn>).mockResolvedValue({
      workspaces: MOCK_WORKSPACES,
      total: 2,
    });
  });

  it("resolves a single workspace by id", async () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useWorkspace("ws-2"), { wrapper: Wrapper });
    await waitFor(() => expect(result.current?.name).toBe("Beta"));
  });

  it("returns null for a null id", () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useWorkspace(null), { wrapper: Wrapper });
    expect(result.current).toBeNull();
  });

  it("returns null for an unknown id", async () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useWorkspace("ws-missing"), { wrapper: Wrapper });
    await waitFor(() => expect(listWorkspaces).toHaveBeenCalled());
    expect(result.current).toBeNull();
  });
});
