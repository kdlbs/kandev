import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { DEFAULT_TASKS_LIST_GROUP, DEFAULT_TASKS_LIST_SORT } from "@/lib/tasks/tasks-list-options";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type Task,
} from "@/lib/types/http";
import { TasksListView, type TasksListViewProps } from "./tasks-list-view";

afterEach(cleanup);

function message(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type: "message",
    created_at: "2026-05-02T00:00:00Z",
    ...overrides,
  };
}

function makeTask(overrides: Partial<Task>): Task {
  return {
    id: toTaskId("task-1"),
    title: "A task",
    state: "WAITING_FOR_INPUT",
    workflow_step_id: "step-1",
    primary_session_id: toSessionId("session-1"),
    ...overrides,
  } as Task;
}

function props(tasks: Task[]): TasksListViewProps {
  return {
    total: tasks.length,
    showArchived: false,
    setShowArchived: () => undefined,
    tasksListSort: DEFAULT_TASKS_LIST_SORT,
    onTasksListSortChange: () => undefined,
    tasksListGroup: DEFAULT_TASKS_LIST_GROUP,
    onTasksListGroupChange: () => undefined,
    tasks,
    workflows: [],
    repositories: [],
    pageCount: 1,
    pagination: { pageIndex: 0, pageSize: 25 },
    setPagination: () => undefined,
    isLoading: false,
    handleRowClick: () => undefined,
    deletingTaskId: null,
    handleArchive: async () => undefined,
    handleUnarchive: async () => undefined,
    handleDelete: async () => undefined,
  };
}

function renderList(task: Task, messagesBySession: Record<string, Message[]> = {}) {
  return render(
    <StateProvider initialState={{ messages: { bySession: messagesBySession, metaBySession: {} } }}>
      <TooltipProvider>
        <TasksListView {...props([task])} />
      </TooltipProvider>
    </StateProvider>,
  );
}

describe("TasksListView row — waiting-for-input parity (§spec:waiting-for-input-parity)", () => {
  it("renders the message-question for a pending clarification (path previously disabled)", () => {
    const { container } = renderList(makeTask({}), {
      "session-1": [message({ type: "clarification_request", metadata: { status: "pending" } })],
    });
    expect(container.querySelector(".tabler-icon-message-question")).not.toBeNull();
    expect(container.querySelector(".tabler-icon-check")).toBeNull();
    expect(container.querySelector(".tabler-icon-loader-2")).toBeNull();
  });

  it("renders the shield-question for a pending permission, distinct from done and running", () => {
    const { container } = renderList(makeTask({}), {
      "session-1": [message({ type: "permission_request", metadata: { status: "pending" } })],
    });
    expect(container.querySelector(".tabler-icon-shield-question")).not.toBeNull();
    expect(container.querySelector(".tabler-icon-check")).toBeNull();
    expect(container.querySelector(".tabler-icon-loader-2")).toBeNull();
  });

  it("falls back to the boot snapshot pending action when messages are not loaded", () => {
    const { container } = renderList(
      makeTask({
        primary_session_state: "WAITING_FOR_INPUT",
        primary_session_pending_action: "permission",
      }),
    );
    expect(container.querySelector(".tabler-icon-shield-question")).not.toBeNull();
  });

  it("shows the plain waiting question for a finished turn awaiting a reply", () => {
    const { container } = renderList(makeTask({}), { "session-1": [] });
    expect(container.querySelector(".tabler-icon-message-question")).not.toBeNull();
  });
});
