import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { qk } from "@/lib/query/keys";
import type { Workflow, WorkflowSnapshot } from "@/lib/types/http";
import { useAllWorkflowSnapshots } from "./use-all-workflow-snapshots";

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchWorkflowSnapshot: vi.fn(),
}));

const WORKFLOW_ID = "workflow-1";
const WORKSPACE_ID = "workspace-1";

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

function workflow(): Workflow {
  return {
    id: WORKFLOW_ID,
    workspace_id: WORKSPACE_ID,
    name: "Build",
    sort_order: 0,
    hidden: false,
  } as Workflow;
}

function snapshot(): WorkflowSnapshot {
  return {
    workflow: workflow(),
    steps: [
      {
        id: "step-1",
        workflow_id: WORKFLOW_ID,
        name: "Todo",
        position: 0,
        color: "bg-blue-500",
        allow_manual_move: true,
      },
    ],
    tasks: [
      {
        id: "task-1",
        workspace_id: WORKSPACE_ID,
        workflow_id: WORKFLOW_ID,
        workflow_step_id: "step-1",
        position: 0,
        title: "Cached task",
        description: "from query",
        state: "TODO",
        priority: 0,
        repositories: [],
        created_at: "2026-06-24T00:00:00Z",
        updated_at: "2026-06-24T00:00:00Z",
      },
    ],
  } as unknown as WorkflowSnapshot;
}

describe("useAllWorkflowSnapshots query ownership", () => {
  afterEach(() => {
    cleanup();
  });

  it("returns converted snapshots from Query cache without a Zustand store", () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.workflows.all(WORKSPACE_ID, { includeHidden: true }), [workflow()]);
    queryClient.setQueryData(qk.workflows.snapshot(WORKFLOW_ID), snapshot());

    const { result } = renderHook(() => useAllWorkflowSnapshots(WORKSPACE_ID), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current).toMatchObject({
      isLoading: false,
      snapshots: {
        [WORKFLOW_ID]: {
          workflowId: WORKFLOW_ID,
          workflowName: "Build",
          tasks: [
            expect.objectContaining({
              id: "task-1",
              title: "Cached task",
            }),
          ],
        },
      },
    });
  });
});
