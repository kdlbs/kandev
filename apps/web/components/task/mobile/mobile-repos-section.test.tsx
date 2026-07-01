import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import type { AppState } from "@/lib/state/store";
import { repositoryId, taskId, workflowId, workspaceId } from "@/lib/types/ids";
import type { Task } from "@/lib/types/http";
import { useTaskRepoCount } from "./mobile-repos-section";

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(),
}));

const WORKSPACE_ID = workspaceId("workspace-1");
const WORKFLOW_ID = workflowId("workflow-1");
const TASK_ID = taskId("task-1");
const CREATED_AT = "2026-06-24T00:00:00Z";

function createQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
}

function wrapperFor(queryClient: QueryClient) {
  const initialState = {
    kanban: {
      workflowId: null,
      steps: [],
      tasks: [],
      isLoading: false,
    },
    kanbanMulti: { snapshots: {} },
  } as Partial<AppState>;

  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <StateProvider initialState={initialState}>{children}</StateProvider>
      </QueryClientProvider>
    );
  };
}

function task(): Task {
  return {
    id: TASK_ID,
    workspace_id: WORKSPACE_ID,
    workflow_id: WORKFLOW_ID,
    workflow_step_id: "step-1",
    position: 0,
    title: "Multi repo task",
    description: "",
    state: "TODO",
    priority: 0,
    repositories: [
      {
        id: "task-repo-front",
        task_id: TASK_ID,
        repository_id: repositoryId("repo-front"),
        base_branch: "main",
        position: 0,
        created_at: CREATED_AT,
        updated_at: CREATED_AT,
      },
      {
        id: "task-repo-back",
        task_id: TASK_ID,
        repository_id: repositoryId("repo-back"),
        base_branch: "release",
        position: 1,
        created_at: CREATED_AT,
        updated_at: CREATED_AT,
      },
    ],
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

describe("useTaskRepoCount", () => {
  afterEach(() => {
    cleanup();
  });

  it("uses task detail Query data when legacy kanban mirrors are empty", () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.tasks.detail(TASK_ID), task());

    const { result } = renderHook(() => useTaskRepoCount(TASK_ID), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current).toBe(2);
  });
});
