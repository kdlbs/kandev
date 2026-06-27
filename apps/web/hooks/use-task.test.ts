import { createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import { qk } from "@/lib/query/keys";
import { taskId, workspaceId, workflowId } from "@/lib/types/ids";
import type { Task } from "@/lib/types/http";
import { useTask } from "./use-task";

const fetchTaskMock = vi.fn();

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: (...args: unknown[]) => fetchTaskMock(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

function makeTask(id: string, title: string): Task {
  return {
    id: taskId(id),
    workspace_id: workspaceId("ws-1"),
    workflow_id: workflowId("wf-1"),
    workflow_step_id: "step-1",
    position: 1,
    title,
    description: "Task description",
    state: "CREATED",
    priority: 0,
    repositories: [],
    created_at: "2026-06-24T00:00:00Z",
    updated_at: "2026-06-24T00:00:00Z",
  };
}

function createQueryWrapper(tasks: Task[]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  for (const task of tasks) {
    client.setQueryData(qk.tasks.detail(task.id), task);
  }
  return function QueryWrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client }, children);
  };
}

describe("useTask", () => {
  it("resolves a task from the Query cache without requiring the Zustand store", () => {
    const task = makeTask("task-1", "Cached task");
    const { result } = renderHook(() => useTask("task-1"), {
      wrapper: createQueryWrapper([task]),
    });

    expect(result.current?.id).toBe("task-1");
    expect(result.current?.title).toBe("Cached task");
    expect(result.current?.workflowStepId).toBe("step-1");
  });
});

describe("useTaskById", () => {
  it("resolves a task from the Query cache without requiring the Zustand store", () => {
    const task = makeTask("task-2", "Sender task");
    const { result } = renderHook(() => useTaskById("task-2"), {
      wrapper: createQueryWrapper([task]),
    });

    expect(result.current?.id).toBe("task-2");
    expect(result.current?.title).toBe("Sender task");
  });
});
