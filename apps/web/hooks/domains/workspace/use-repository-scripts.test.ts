import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useRepositoryScripts } from "./use-repository-scripts";

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listRepositoryScripts: vi.fn(),
}));

import { listRepositoryScripts } from "@/lib/api/domains/workspace-api";

const MOCK_SCRIPTS = [
  {
    id: "script-1",
    repository_id: "repo-1",
    name: "Build",
    command: "npm run build",
    position: 0,
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

describe("useRepositoryScripts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (listRepositoryScripts as ReturnType<typeof vi.fn>).mockResolvedValue({
      scripts: MOCK_SCRIPTS,
    });
  });

  it("fetches scripts for a repository", async () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useRepositoryScripts("repo-1"), { wrapper: Wrapper });
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.scripts).toEqual(MOCK_SCRIPTS);
    expect(result.current.isLoaded).toBe(true);
    expect(listRepositoryScripts).toHaveBeenCalledWith("repo-1");
  });

  it("returns empty scripts when repositoryId is null", () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useRepositoryScripts(null), { wrapper: Wrapper });
    expect(result.current.scripts).toEqual([]);
    expect(result.current.isLoading).toBe(false);
    expect(listRepositoryScripts).not.toHaveBeenCalled();
  });

  it("does not fetch when enabled=false", () => {
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useRepositoryScripts("repo-1", false), {
      wrapper: Wrapper,
    });
    expect(result.current.scripts).toEqual([]);
    expect(listRepositoryScripts).not.toHaveBeenCalled();
  });

  it("isLoaded is false while loading", () => {
    (listRepositoryScripts as ReturnType<typeof vi.fn>).mockImplementation(
      () => new Promise(() => {}), // never resolves
    );
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useRepositoryScripts("repo-1"), { wrapper: Wrapper });
    expect(result.current.isLoaded).toBe(false);
    expect(result.current.isLoading).toBe(true);
  });
});
