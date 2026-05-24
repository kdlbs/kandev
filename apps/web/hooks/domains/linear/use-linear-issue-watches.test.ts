import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useLinearIssueWatches } from "./use-linear-issue-watches";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { qk } from "@/lib/query/keys";
import type { LinearIssueWatch } from "@/lib/types/linear";

vi.mock("@/lib/api/domains/linear-api", () => ({
  listLinearIssueWatches: vi.fn(),
  createLinearIssueWatch: vi.fn(),
  updateLinearIssueWatch: vi.fn(),
  deleteLinearIssueWatch: vi.fn(),
  triggerLinearIssueWatch: vi.fn(),
}));

import {
  listLinearIssueWatches,
  createLinearIssueWatch,
  updateLinearIssueWatch,
  deleteLinearIssueWatch,
  triggerLinearIssueWatch,
} from "@/lib/api/domains/linear-api";

const mockList = vi.mocked(listLinearIssueWatches);
const mockCreate = vi.mocked(createLinearIssueWatch);
const mockUpdate = vi.mocked(updateLinearIssueWatch);
const mockDelete = vi.mocked(deleteLinearIssueWatch);
const mockTrigger = vi.mocked(triggerLinearIssueWatch);

function makeWrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client }, children);
  };
}

export function makeWatch(id: string, overrides: Partial<LinearIssueWatch> = {}): LinearIssueWatch {
  return {
    id,
    workspaceId: "ws-1",
    workflowId: "wf-1",
    workflowStepId: "step-1",
    filter: { teamKey: "ENG" },
    agentProfileId: "",
    executorProfileId: "",
    prompt: "",
    enabled: true,
    pollIntervalSeconds: 300,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

async function renderAndWaitLoaded(workspaceId?: string | null, client?: QueryClient) {
  const qc = client ?? createTestQueryClient();
  mockList.mockResolvedValue([]);
  const hook = renderHook(() => useLinearIssueWatches(workspaceId), {
    wrapper: makeWrapper(qc),
  });
  await waitFor(() => expect(hook.result.current.loaded).toBe(true));
  return { ...hook, qc };
}

describe("useLinearIssueWatches — fetching", () => {
  let client: QueryClient;

  beforeEach(() => {
    client = createTestQueryClient();
    vi.clearAllMocks();
  });

  it("does not fetch when workspaceId is null", () => {
    mockList.mockResolvedValue([]);
    renderHook(() => useLinearIssueWatches(null), { wrapper: makeWrapper(client) });
    expect(mockList).not.toHaveBeenCalled();
  });

  it("returns empty items while loading", () => {
    mockList.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useLinearIssueWatches("ws-1"), {
      wrapper: makeWrapper(client),
    });
    expect(result.current.items).toEqual([]);
  });

  it("fetches scoped watches when workspaceId is a string", async () => {
    const watches = [makeWatch("a"), makeWatch("b")];
    mockList.mockResolvedValue(watches);
    const { result } = renderHook(() => useLinearIssueWatches("ws-1"), {
      wrapper: makeWrapper(client),
    });
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.items).toEqual(watches);
    expect(mockList).toHaveBeenCalledWith("ws-1", { cache: "no-store" });
  });

  it("fetches all-workspace watches when workspaceId is undefined", async () => {
    const watches = [makeWatch("a")];
    mockList.mockResolvedValue(watches);
    const { result } = renderHook(() => useLinearIssueWatches(), {
      wrapper: makeWrapper(client),
    });
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.items).toEqual(watches);
    expect(mockList).toHaveBeenCalledWith(undefined, { cache: "no-store" });
  });

  it("reads from the cache when data is pre-seeded", async () => {
    const watches = [makeWatch("cached")];
    client.setQueryData(qk.linear.watches("ws-1"), watches);
    mockList.mockResolvedValue(watches);
    const { result } = renderHook(() => useLinearIssueWatches("ws-1"), {
      wrapper: makeWrapper(client),
    });
    expect(result.current.items).toEqual(watches);
  });
});

describe("useLinearIssueWatches — mutations", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("create calls createLinearIssueWatch and invalidates the query", async () => {
    const client = createTestQueryClient();
    mockList.mockResolvedValue([makeWatch("a")]);
    const newWatch = makeWatch("new");
    mockCreate.mockResolvedValue(newWatch);
    const { result } = renderHook(() => useLinearIssueWatches("ws-1"), {
      wrapper: makeWrapper(client),
    });
    await waitFor(() => expect(result.current.loaded).toBe(true));
    mockList.mockResolvedValue([makeWatch("a"), newWatch]);
    await act(async () => {
      await result.current.create({
        workspaceId: "ws-1",
        workflowId: "wf-1",
        workflowStepId: "step-1",
        filter: { teamKey: "ENG" },
        agentProfileId: "",
        executorProfileId: "",
        prompt: "",
        enabled: true,
        pollIntervalSeconds: 300,
      });
    });
    expect(mockCreate).toHaveBeenCalledOnce();
    await waitFor(() => expect(mockList).toHaveBeenCalledTimes(2));
  });

  it("update calls updateLinearIssueWatch with correct args", async () => {
    const client = createTestQueryClient();
    mockList.mockResolvedValue([makeWatch("a")]);
    mockUpdate.mockResolvedValue(makeWatch("a", { enabled: false }));
    const { result } = renderHook(() => useLinearIssueWatches("ws-1"), {
      wrapper: makeWrapper(client),
    });
    await waitFor(() => expect(result.current.loaded).toBe(true));
    await act(async () => {
      await result.current.update("a", { enabled: false });
    });
    expect(mockUpdate).toHaveBeenCalledWith("ws-1", "a", { enabled: false });
  });

  it("update uses rowWorkspaceId for per-row IDOR guard", async () => {
    const { result } = await renderAndWaitLoaded();
    mockUpdate.mockResolvedValue(makeWatch("a", { workspaceId: "ws-2" }));
    await act(async () => {
      await result.current.update("a", { enabled: false }, "ws-2");
    });
    expect(mockUpdate).toHaveBeenCalledWith("ws-2", "a", { enabled: false });
  });

  it("update throws synchronously when no workspaceId is resolvable", async () => {
    const { result } = await renderAndWaitLoaded();
    expect(() => result.current.update("a", { enabled: false })).toThrow("workspaceId required");
  });

  it("remove calls deleteLinearIssueWatch with correct args", async () => {
    const client = createTestQueryClient();
    mockList.mockResolvedValue([makeWatch("a")]);
    mockDelete.mockResolvedValue({ deleted: true });
    const { result } = renderHook(() => useLinearIssueWatches("ws-1"), {
      wrapper: makeWrapper(client),
    });
    await waitFor(() => expect(result.current.loaded).toBe(true));
    await act(async () => {
      await result.current.remove("a");
    });
    expect(mockDelete).toHaveBeenCalledWith("ws-1", "a");
  });

  it("remove throws synchronously when no workspaceId is resolvable", async () => {
    const { result } = await renderAndWaitLoaded();
    expect(() => result.current.remove("a")).toThrow("workspaceId required");
  });

  it("trigger calls triggerLinearIssueWatch and returns result", async () => {
    const client = createTestQueryClient();
    mockList.mockResolvedValue([makeWatch("a")]);
    mockTrigger.mockResolvedValue({ newIssues: 3 });
    const { result } = renderHook(() => useLinearIssueWatches("ws-1"), {
      wrapper: makeWrapper(client),
    });
    await waitFor(() => expect(result.current.loaded).toBe(true));
    let res: { newIssues: number } | undefined;
    await act(async () => {
      res = await result.current.trigger("a");
    });
    expect(mockTrigger).toHaveBeenCalledWith("ws-1", "a");
    expect(res?.newIssues).toBe(3);
  });

  it("trigger uses rowWorkspaceId for IDOR guard", async () => {
    const { result } = await renderAndWaitLoaded();
    mockTrigger.mockResolvedValue({ newIssues: 0 });
    await act(async () => {
      await result.current.trigger("a", "ws-2");
    });
    expect(mockTrigger).toHaveBeenCalledWith("ws-2", "a");
  });

  it("trigger throws synchronously when no workspaceId is resolvable", async () => {
    const { result } = await renderAndWaitLoaded();
    expect(() => result.current.trigger("a")).toThrow("workspaceId required");
  });
});
