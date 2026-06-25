import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listWorkflows } from "@/lib/api/domains/kanban-api";
import { qk } from "@/lib/query/keys";
import type { Workflow } from "@/lib/types/http";
import { useWorkflows } from "./use-workflows";

const WORKSPACE_ID = "workspace-1";

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
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
}

function workflow(id: string, name: string, sortOrder = 0): Workflow {
  return {
    id,
    workspace_id: WORKSPACE_ID,
    name,
    description: null,
    sort_order: sortOrder,
    hidden: false,
  } as Workflow;
}

describe("useWorkflows", () => {
  beforeEach(() => {
    vi.mocked(listWorkflows).mockReset();
  });

  afterEach(() => {
    cleanup();
  });

  it("reads workspace workflows from the Query cache without a Zustand store", () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_ID, { includeHidden: true }), [
      workflow("workflow-1", "Build", 10),
    ]);

    const { result } = renderHook(() => useWorkflows(WORKSPACE_ID), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current).toEqual({
      workflows: [
        expect.objectContaining({
          id: "workflow-1",
          workspaceId: WORKSPACE_ID,
          name: "Build",
          sortOrder: 10,
        }),
      ],
      isLoading: false,
    });
  });

  it("cold-loads workspace workflows through the query option", async () => {
    const workflows = [workflow("workflow-1", "Build")];
    vi.mocked(listWorkflows).mockResolvedValue({ workflows, total: 1 });
    const queryClient = createQueryClient();

    const { result } = renderHook(() => useWorkflows(WORKSPACE_ID), {
      wrapper: wrapperFor(queryClient),
    });

    await waitFor(() => expect(result.current.workflows).toHaveLength(1));

    expect(listWorkflows).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.objectContaining({ includeHidden: true }),
    );
    expect(queryClient.getQueryData(qk.workflows.all(WORKSPACE_ID, { includeHidden: true }))).toBe(
      workflows,
    );
  });
});
