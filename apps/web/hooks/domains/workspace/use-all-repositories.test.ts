import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useAllRepositories } from "./use-all-repositories";
import { qk } from "@/lib/query/keys";

vi.mock("@/lib/api/domains/workspace-api", () => ({
  listWorkspaces: vi.fn(),
  listRepositories: vi.fn(),
}));

import { listWorkspaces, listRepositories } from "@/lib/api/domains/workspace-api";

const WORKSPACES = [
  { id: "ws-1", name: "Alpha", owner_id: "u", created_at: "", updated_at: "" },
  { id: "ws-2", name: "Beta", owner_id: "u", created_at: "", updated_at: "" },
];

function repo(id: string, ws: string) {
  return { id, workspace_id: ws, name: id, source_type: "github", local_path: `/tmp/${id}` };
}

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: qc }, children);
  }
  return { qc, Wrapper };
}

describe("useAllRepositories", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (listWorkspaces as ReturnType<typeof vi.fn>).mockResolvedValue({
      workspaces: WORKSPACES,
      total: 2,
    });
  });

  it("observe-only (enabled=false) returns what is already cached without fetching", () => {
    const { qc, Wrapper } = makeWrapper();
    qc.setQueryData(qk.workspaces.all(), { workspaces: WORKSPACES, total: 2 });
    qc.setQueryData(qk.workspaces.repos("ws-1"), { repositories: [repo("r-1", "ws-1")], total: 1 });

    const { result } = renderHook(() => useAllRepositories(false), { wrapper: Wrapper });
    expect(result.current.repositories.map((r) => r.id)).toEqual(["r-1"]);
    expect(result.current.byWorkspaceId["ws-1"]?.map((r) => r.id)).toEqual(["r-1"]);
    expect(listWorkspaces).not.toHaveBeenCalled();
    expect(listRepositories).not.toHaveBeenCalled();
  });

  it("fetches per-workspace repos when enabled and merges into a flat list + map", async () => {
    (listRepositories as ReturnType<typeof vi.fn>).mockImplementation((wsId: string) =>
      Promise.resolve({ repositories: [repo(`r-${wsId}`, wsId)], total: 1 }),
    );
    const { Wrapper } = makeWrapper();
    const { result } = renderHook(() => useAllRepositories(true), { wrapper: Wrapper });

    await waitFor(() => expect(result.current.repositories.length).toBe(2));
    expect(result.current.repositories.map((r) => r.id).sort()).toEqual(["r-ws-1", "r-ws-2"]);
    expect(result.current.byWorkspaceId["ws-1"]?.[0]?.id).toBe("r-ws-1");
    expect(result.current.byWorkspaceId["ws-2"]?.[0]?.id).toBe("r-ws-2");
  });
});
