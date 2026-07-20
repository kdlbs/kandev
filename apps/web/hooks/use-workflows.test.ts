import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { qk } from "@/lib/query/keys";
import { workflowId, workspaceId as toWorkspaceId, type Workflow } from "@/lib/types/http";
import { useEnsureWorkspaceWorkflows } from "./use-workflows";

type MockState = {
  workspaces: { activeId: string | null };
};

let mockState: MockState = {
  workspaces: { activeId: null },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: MockState) => unknown) => selector(mockState),
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  listWorkflows: vi.fn(),
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
    return createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

function workflow(id: string, workspaceId: string): Workflow {
  return {
    id: workflowId(id),
    workspace_id: toWorkspaceId(workspaceId),
    name: id,
    description: null,
    sort_order: 0,
    hidden: false,
    created_at: "",
    updated_at: "",
  };
}

describe("useEnsureWorkspaceWorkflows", () => {
  beforeEach(() => {
    vi.mocked(listWorkflows).mockReset();
    vi.mocked(listWorkflows).mockResolvedValue({ workflows: [], total: 0 });
    mockState = { workspaces: { activeId: "ws-A" } };
  });

  afterEach(() => {
    cleanup();
  });

  it("warms the active workspace workflow query on mount", async () => {
    const queryClient = createQueryClient();
    const workflows = [workflow("wf-A", "ws-A")];
    vi.mocked(listWorkflows).mockResolvedValueOnce({ workflows, total: 1 });

    renderHook(() => useEnsureWorkspaceWorkflows(), { wrapper: wrapperFor(queryClient) });

    await waitFor(() =>
      expect(listWorkflows).toHaveBeenCalledWith(
        "ws-A",
        expect.objectContaining({ includeHidden: true }),
      ),
    );
    await waitFor(() =>
      expect(queryClient.getQueryData(qk.workflows.all("ws-A", { includeHidden: true }))).toEqual(
        workflows,
      ),
    );
  });

  it("warms the next workspace when the active workspace changes", async () => {
    const queryClient = createQueryClient();
    vi.mocked(listWorkflows)
      .mockResolvedValueOnce({ workflows: [workflow("wf-A", "ws-A")], total: 1 })
      .mockResolvedValueOnce({ workflows: [workflow("wf-B", "ws-B")], total: 1 });

    const { rerender } = renderHook(() => useEnsureWorkspaceWorkflows(), {
      wrapper: wrapperFor(queryClient),
    });
    await waitFor(() => expect(listWorkflows).toHaveBeenCalledWith("ws-A", expect.anything()));

    mockState = { workspaces: { activeId: "ws-B" } };
    rerender();

    await waitFor(() => expect(listWorkflows).toHaveBeenCalledWith("ws-B", expect.anything()));
  });

  it("does not fetch while the active workspace is unresolved", () => {
    mockState = { workspaces: { activeId: null } };
    const queryClient = createQueryClient();

    renderHook(() => useEnsureWorkspaceWorkflows(), { wrapper: wrapperFor(queryClient) });

    expect(listWorkflows).not.toHaveBeenCalled();
  });
});
