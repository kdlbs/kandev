import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, act, cleanup } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { qk } from "@/lib/query/keys";
import {
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type Workflow,
} from "@/lib/types/http";
import type { WorkflowItem } from "@/lib/state/slices";

vi.mock("@/lib/api/domains/kanban-api", () => ({
  listWorkflows: vi.fn(),
}));

import { useWorkflowSettings } from "./use-workflow-settings";

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
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

function setWorkflowCache(queryClient: QueryClient, items: WorkflowItem[]) {
  const grouped = new Map<string, WorkflowItem[]>();
  for (const item of items) {
    grouped.set(item.workspaceId, [...(grouped.get(item.workspaceId) ?? []), item]);
  }
  const workspaceIds = new Set([
    ...queryClient
      .getQueryCache()
      .findAll({ queryKey: ["workflows"] })
      .map((query) => query.queryKey[1])
      .filter((id): id is string => typeof id === "string"),
    ...grouped.keys(),
  ]);
  for (const workspaceId of workspaceIds) {
    queryClient.setQueryData(
      qk.workflows.all(workspaceId, { includeHidden: true }),
      grouped.get(workspaceId) ?? [],
    );
  }
}

const wf = (id: string, wsId: string, name: string): Workflow => ({
  id: toWorkflowId(id),
  workspace_id: toWorkspaceId(wsId),
  name,
  description: "",
  created_at: "",
  updated_at: "",
});

const NAME_A1 = "Workflow A1";
const NAME_B1 = "Workflow B1";
const CACHE_A1: WorkflowItem = { id: "wf-a1", workspaceId: "ws-a", name: NAME_A1 };
const CACHE_B1: WorkflowItem = { id: "wf-b1", workspaceId: "ws-b", name: NAME_B1 };

beforeEach(() => {
  vi.mocked(listWorkflows).mockReset();
  vi.mocked(listWorkflows).mockResolvedValue({ workflows: [], total: 0 });
  cleanup();
});

describe("useWorkflowSettings", () => {
  it("does not include workflows from other workspaces present in the cache", () => {
    const queryClient = createQueryClient();
    setWorkflowCache(queryClient, [CACHE_A1]);

    // We render the settings hook for workspace B with no initial workflows
    const { result } = renderHook(() => useWorkflowSettings([], "ws-b"), {
      wrapper: wrapperFor(queryClient),
    });

    // The leaked workflow from workspace A must not appear in B's list
    expect(result.current.workflowItems).toHaveLength(0);
    expect(result.current.savedWorkflowItems).toHaveLength(0);
  });

  it("adds workflows from the cache that belong to the current workspace", () => {
    const queryClient = createQueryClient();
    setWorkflowCache(queryClient, [CACHE_A1, CACHE_B1]);

    const { result } = renderHook(() => useWorkflowSettings([], "ws-b"), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });

  it("does not remove a workspace's workflows when an unrelated workspace's entries are added/removed in the cache", () => {
    // Initial: workspace B has one saved workflow from SSR
    const initial = [wf("wf-b1", "ws-b", NAME_B1)];
    const queryClient = createQueryClient();
    setWorkflowCache(queryClient, [CACHE_B1]);

    const { result, rerender } = renderHook(() => useWorkflowSettings(initial, "ws-b"), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);

    // Workspace A workflow is added to the cache (e.g. invalidation from another route)
    act(() => {
      setWorkflowCache(queryClient, [CACHE_B1, CACHE_A1]);
    });
    rerender();
    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);

    // Workspace A workflow is removed from the cache — must not affect B's list
    act(() => {
      setWorkflowCache(queryClient, [CACHE_B1]);
    });
    rerender();
    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });

  it("falls back to the unscoped cache when no workspaceId is provided", () => {
    const queryClient = createQueryClient();
    setWorkflowCache(queryClient, [CACHE_A1, CACHE_B1]);

    const { result } = renderHook(() => useWorkflowSettings([]), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.workflowItems.map((w) => w.id).sort()).toEqual(["wf-a1", "wf-b1"]);
  });
});

describe("useWorkflowSettings cache updates", () => {
  it("syncs name updates from the cache within the current workspace", () => {
    const initial = [wf("wf-b1", "ws-b", NAME_B1)];
    const queryClient = createQueryClient();
    setWorkflowCache(queryClient, [CACHE_B1]);

    const { result, rerender } = renderHook(() => useWorkflowSettings(initial, "ws-b"), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.workflowItems[0].name).toEqual(NAME_B1);

    act(() => {
      setWorkflowCache(queryClient, [{ id: "wf-b1", workspaceId: "ws-b", name: "Renamed B1" }]);
    });
    rerender();

    expect(result.current.workflowItems[0].name).toEqual("Renamed B1");
  });

  it("does not overwrite a dirty name when the query cache refreshes", () => {
    const initial = [wf("wf-b1", "ws-b", NAME_B1)];
    const queryClient = createQueryClient();
    setWorkflowCache(queryClient, [CACHE_B1]);
    const { result, rerender } = renderHook(() => useWorkflowSettings(initial, "ws-b"), {
      wrapper: wrapperFor(queryClient),
    });

    act(() => {
      result.current.setWorkflowItems((items) =>
        items.map((item) => ({ ...item, name: "Unsaved local name" })),
      );
    });
    act(() => {
      setWorkflowCache(queryClient, [{ ...CACHE_B1, name: "Remote name" }]);
    });
    rerender();

    expect(result.current.workflowItems[0].name).toBe("Unsaved local name");
    expect(result.current.savedWorkflowItems[0].name).toBe("Remote name");
    expect(result.current.isWorkflowDirty(result.current.workflowItems[0])).toBe(true);
  });
});

describe("useWorkflowSettings visibility", () => {
  it("excludes hidden system workflows from the settings list", () => {
    // System workflows like "Improve Kandev" live in Query with
    // hidden=true so the kanban can resolve task references, but they must
    // never appear in the management UI.
    const HIDDEN_SYSTEM: WorkflowItem = {
      id: "wf-improve-kandev",
      workspaceId: "ws-b",
      name: "Improve Kandev",
      hidden: true,
    };
    const queryClient = createQueryClient();
    setWorkflowCache(queryClient, [CACHE_B1, HIDDEN_SYSTEM]);

    const { result } = renderHook(() => useWorkflowSettings([], "ws-b"), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
    expect(result.current.savedWorkflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });

  it("excludes office-style workflows from the settings list", () => {
    const OFFICE_WORKFLOW: WorkflowItem = {
      id: "wf-office",
      workspaceId: "ws-b",
      name: "Office Only Workflow",
      style: "office",
    };
    const queryClient = createQueryClient();
    setWorkflowCache(queryClient, [CACHE_B1, OFFICE_WORKFLOW]);

    const { result } = renderHook(() => useWorkflowSettings([], "ws-b"), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
    expect(result.current.savedWorkflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });

  it("drops a workflow from the settings list once it becomes hidden", () => {
    const initial = [wf("wf-b1", "ws-b", NAME_B1)];
    const queryClient = createQueryClient();
    setWorkflowCache(queryClient, [CACHE_B1]);

    const { result, rerender } = renderHook(() => useWorkflowSettings(initial, "ws-b"), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);

    // Backend flips hidden=true (e.g. healing the improve-kandev record).
    act(() => {
      setWorkflowCache(queryClient, [{ ...CACHE_B1, hidden: true }]);
    });
    rerender();

    expect(result.current.workflowItems.map((w) => w.id)).toEqual([]);
  });

  it("starts scoping cache entries once a workspaceId becomes defined", () => {
    const queryClient = createQueryClient();
    setWorkflowCache(queryClient, [CACHE_A1, CACHE_B1]);

    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId?: string }) => useWorkflowSettings([], workspaceId),
      {
        initialProps: { workspaceId: undefined as string | undefined },
        wrapper: wrapperFor(queryClient),
      },
    );

    // No workspaceId → unscoped fallback shows both
    expect(result.current.workflowItems.map((w) => w.id).sort()).toEqual(["wf-a1", "wf-b1"]);

    act(() => {
      rerender({ workspaceId: "ws-b" });
    });

    // Once scoped to B, A's workflow is dropped
    expect(result.current.workflowItems.map((w) => w.id)).toEqual(["wf-b1"]);
  });
});
