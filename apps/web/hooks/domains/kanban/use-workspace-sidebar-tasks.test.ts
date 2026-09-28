import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Task, SidebarTaskPageResponse } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  state: {
    taskRemoval: {
      pendingTokenByTaskId: {} as Record<string, string>,
      operationsByToken: {} as Record<string, unknown>,
    },
    kanbanMulti: { snapshots: {} as Record<string, unknown> },
    sidebarStatusSummaryByWorkspaceId: {} as Record<string, Record<string, unknown>>,
    workflows: {
      items: [] as Array<{ id: string; workspaceId: string; name: string; hidden?: boolean }>,
    },
    kanban: {
      workflowId: null as string | null,
      steps: [] as Array<{ id: string; title: string; color: string; position: number }>,
    },
    workspaceContextGeneration: 0,
    workspaceContextRead: undefined as unknown,
    requestWorkspaceContextRefresh: vi.fn(),
  },
  page: {
    response: null as SidebarTaskPageResponse | null,
    isLoading: false,
    error: null as string | null,
    refresh: vi.fn(),
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));
vi.mock("@/hooks/domains/kanban/use-sidebar-task-page", () => ({
  useSidebarTaskPage: () => ({
    ...mocks.page,
    view: { id: "view-1", group: "none" },
  }),
}));

import { useWorkspaceSidebarTasks } from "./use-workspace-sidebar-tasks";

function task(id: string, overrides: Partial<Task> = {}): Task {
  return {
    id,
    workspace_id: "ws-1",
    workflow_id: "wf-1",
    workflow_step_id: "step-1",
    title: id,
    description: "",
    state: "TODO",
    priority: "medium",
    position: 0,
    created_at: "2026-09-26T10:00:00Z",
    updated_at: "2026-09-26T10:00:00Z",
    ...overrides,
  } as Task;
}

function setPageTasks(tasks: Task[]) {
  mocks.page.response = {
    query_key: "query-1",
    page: 1,
    page_size: 100,
    total_entries: tasks.length + 1,
    total_tasks: tasks.length,
    total_visible_tasks: tasks.length,
    has_previous: false,
    has_next: false,
    entries: [
      { kind: "group", group_key: "__all__", group_label: "__all__" },
      ...tasks.map((item) => ({ kind: "task" as const, task_id: item.id, task: item })),
    ],
  };
}

describe("useWorkspaceSidebarTasks", () => {
  beforeEach(() => {
    mocks.state.taskRemoval = { pendingTokenByTaskId: {}, operationsByToken: {} };
    mocks.state.kanbanMulti = { snapshots: {} };
    mocks.state.sidebarStatusSummaryByWorkspaceId = {};
    mocks.state.workflows = { items: [{ id: "wf-1", workspaceId: "ws-1", name: "Workflow" }] };
    mocks.state.kanban = {
      workflowId: null,
      steps: [{ id: "step-1", title: "Start", color: "blue", position: 0 }],
    };
    mocks.state.workspaceContextGeneration = 0;
    mocks.state.workspaceContextRead = undefined;
    mocks.page.response = null;
    mocks.page.isLoading = false;
    mocks.page.error = null;
    vi.clearAllMocks();
  });

  it("uses only the bounded page and does not leak workspace snapshot tasks", () => {
    setPageTasks([task("page-a"), task("page-b")]);
    mocks.state.kanbanMulti.snapshots = {
      "wf-1": {
        workflowId: "wf-1",
        workflowName: "Workflow",
        steps: [{ id: "step-1", title: "Start", color: "blue", position: 0 }],
        tasks: [task("off-page-task")],
      },
    };

    const { result } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));

    expect(result.current.allTasks.map((item) => item.id)).toEqual(["page-a", "page-b"]);
    expect(result.current.allTasks.map((item) => item._workflowId)).toEqual(["wf-1", "wf-1"]);
    expect(result.current.allTasks).toHaveLength(2);
  });

  it("overlays newer live status summaries on visible page rows", () => {
    setPageTasks([task("page-a", { status_summary: { revision: 1, updated_at: "old" } })]);
    mocks.state.sidebarStatusSummaryByWorkspaceId = {
      "ws-1": {
        "page-a": { revision: 2, updated_at: "new", pending_action: "clarification" },
      },
    };

    const { result } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));

    expect(result.current.allTasks[0]?.statusSummary).toMatchObject({
      revision: 2,
      pending_action: "clarification",
    });
  });

  it("keeps pending archive state for current rows and adopts the next accepted page", () => {
    setPageTasks([task("archive-target"), task("sibling")]);
    const { result, rerender } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));

    mocks.state.taskRemoval = {
      pendingTokenByTaskId: { "archive-target": "token-1" },
      operationsByToken: {
        "token-1": { action: "archive", workspaceId: "ws-1" },
      },
    };
    rerender();
    expect(result.current.allTasks.map((item) => item.id)).toEqual(["archive-target", "sibling"]);
    expect(result.current.pendingArchiveTaskIds).toEqual(new Set(["archive-target"]));

    setPageTasks([task("sibling")]);
    mocks.state.taskRemoval = { pendingTokenByTaskId: {}, operationsByToken: {} };
    rerender();
    expect(result.current.allTasks.map((item) => item.id)).toEqual(["sibling"]);
    expect(result.current.pendingArchiveTaskIds).toEqual(new Set());
  });

  it("uses queue position computed across tasks outside the current view page", () => {
    const queuedTask = task("queued-filtered", { queued_for_step_id: "step-1" });
    mocks.page.response = {
      query_key: "query-wip",
      page: 1,
      page_size: 100,
      total_entries: 2,
      total_tasks: 1,
      total_visible_tasks: 1,
      has_previous: false,
      has_next: false,
      entries: [
        { kind: "group", group_key: "__all__", group_label: "__all__" },
        {
          kind: "task",
          task_id: queuedTask.id,
          task: queuedTask,
          workflow_step_name: "Start",
          wip_queue_position: 3,
          wip_queue_total: 3,
        } as unknown as SidebarTaskPageResponse["entries"][number],
      ],
    };

    const { result } = renderHook(() => useWorkspaceSidebarTasks("ws-1"));

    expect(result.current.wipQueueByTaskId.get(queuedTask.id)).toEqual({
      position: 3,
      total: 3,
      destinationTitle: "Start",
    });
  });
});
