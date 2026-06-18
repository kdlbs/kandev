import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TaskDetailRoute } from "./task-detail-route";
import type { FetchedSessionData } from "@/lib/ssr/session-page-state";
import { taskId, workspaceId, workflowId } from "@/lib/types/ids";
import type { Task } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  fetchSessionDataForTask: vi.fn(),
}));

vi.mock("@/components/state-hydrator", () => ({
  StateHydrator: () => <div data-testid="state-hydrator" />,
}));

vi.mock("@/app/tasks/[id]/kanban-task-shell", () => ({
  KanbanTaskShell: ({ task, taskId }: { task: Task | null; taskId: string }) => (
    <div
      data-testid="kanban-task-shell"
      data-route-task-id={taskId}
      data-task-id={task?.id ?? ""}
    />
  ),
}));

vi.mock("@/lib/ssr/session-page-state", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/ssr/session-page-state")>();
  return {
    ...actual,
    fetchSessionDataForTask: mocks.fetchSessionDataForTask,
    extractInitialRepositories: vi.fn(() => []),
    extractInitialScripts: vi.fn(() => []),
  };
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

function makeFetchedData(): FetchedSessionData {
  return {
    task: {
      id: taskId("task-1"),
      title: "Task one",
      description: "",
      workspace_id: workspaceId("workspace-1"),
      workflow_id: workflowId("workflow-1"),
      workflow_step_id: "step-1",
      state: "CREATED",
      priority: 0,
      position: 0,
      repositories: [],
      created_at: "2026-06-16T00:00:00Z",
      updated_at: "2026-06-16T00:00:00Z",
    },
    sessionId: "session-1",
    initialState: {},
    initialTerminals: [],
  };
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("TaskDetailRoute", () => {
  it("waits for route data before mounting the task shell", async () => {
    const routeData = deferred<FetchedSessionData>();
    mocks.fetchSessionDataForTask.mockReturnValueOnce(routeData.promise);

    render(<TaskDetailRoute taskId="task-1" />);

    expect(screen.queryByTestId("kanban-task-shell")).toBeNull();

    routeData.resolve(makeFetchedData());

    await waitFor(() => {
      expect(screen.getByTestId("kanban-task-shell").getAttribute("data-task-id")).toBe("task-1");
    });
  });

  it("uses boot route data without fetching again", async () => {
    render(<TaskDetailRoute taskId="task-1" initialData={makeFetchedData()} />);

    expect(mocks.fetchSessionDataForTask).not.toHaveBeenCalled();
    expect(screen.getByTestId("kanban-task-shell").getAttribute("data-task-id")).toBe("task-1");
    expect(screen.getByTestId("state-hydrator")).toBeTruthy();
  });
});
