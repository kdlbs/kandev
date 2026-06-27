import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { qk } from "@/lib/query/keys";
import type { AppState } from "@/lib/state/store";
import { repositoryId, sessionId, taskId, workflowId, workspaceId } from "@/lib/types/ids";
import type { Repository, Task, TaskSession } from "@/lib/types/http";
import { useRepoDisplayName } from "./use-repo-display-name";

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: vi.fn(),
}));

const WORKSPACE_ID = workspaceId("workspace-1");
const WORKFLOW_ID = workflowId("workflow-1");
const TASK_ID = taskId("task-1");
const SESSION_ID = sessionId("session-1");
const REPOSITORY_ID = repositoryId("repo-1");
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
    taskSessions: {
      items: {
        [SESSION_ID]: taskSession(),
      },
    },
  } as Partial<AppState>;

  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <StateProvider initialState={initialState}>{children}</StateProvider>
      </QueryClientProvider>
    );
  };
}

function taskSession(): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    repository_id: REPOSITORY_ID,
    state: "RUNNING",
    started_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

function repository(): Repository {
  return {
    id: REPOSITORY_ID,
    workspace_id: WORKSPACE_ID,
    name: "kandev",
    source_type: "local",
    local_path: "/work/kandev",
    provider: "github",
    provider_repo_id: "repo-1",
    provider_owner: "kdlbs",
    provider_name: "kandev",
    default_branch: "main",
    worktree_branch_prefix: "",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

function task(): Task {
  return {
    id: TASK_ID,
    workspace_id: WORKSPACE_ID,
    workflow_id: WORKFLOW_ID,
    workflow_step_id: "step-1",
    position: 0,
    title: "Single repo task",
    description: "",
    state: "TODO",
    priority: 0,
    repositories: [
      {
        id: "task-repo-1",
        task_id: TASK_ID,
        repository_id: REPOSITORY_ID,
        base_branch: "main",
        position: 0,
        created_at: CREATED_AT,
        updated_at: CREATED_AT,
      },
    ],
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
  };
}

describe("useRepoDisplayName", () => {
  afterEach(() => {
    cleanup();
  });

  it("uses task detail Query data for the primary repo fallback", () => {
    const queryClient = createQueryClient();
    queryClient.setQueryData(qk.tasks.detail(TASK_ID), task());
    queryClient.setQueryData(qk.workspaces.repositories(WORKSPACE_ID), [repository()]);

    const { result } = renderHook(() => useRepoDisplayName(SESSION_ID), {
      wrapper: wrapperFor(queryClient),
    });

    expect(result.current("")).toBe("kandev");
  });
});
