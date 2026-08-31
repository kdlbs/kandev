import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CommandPanelLiveTask } from "@/lib/commands/task-result-activity";
import type { Task } from "@/lib/types/http";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";

vi.mock("@kandev/ui/command", () => ({
  CommandEmpty: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  CommandGroup: ({ children }: { children: ReactNode }) => <section>{children}</section>,
  CommandItem: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  CommandShortcut: ({ children }: { children: ReactNode }) => <span>{children}</span>,
}));

vi.mock("@kandev/ui/badge", () => ({
  Badge: ({ children }: { children: ReactNode }) => <span>{children}</span>,
}));

import { CommandsListContent } from "./command-panel-results";

function task(id: string, title: string): Task {
  return {
    id: taskId(id),
    workspace_id: workspaceId("workspace-1"),
    workflow_id: workflowId("workflow-1"),
    workflow_step_id: "step-1",
    position: 0,
    title,
    description: "",
    state: "IN_PROGRESS",
    priority: "medium",
    created_at: "2026-08-24T09:00:00Z",
    updated_at: "2026-08-24T09:00:00Z",
  };
}

function taskResults(tasks: Task[], liveTasksById = new Map<string, CommandPanelLiveTask>()) {
  return (
    <CommandsListContent
      commands={[]}
      grouped={[]}
      search="task"
      onSelect={vi.fn()}
      taskResults={tasks}
      isSearching={false}
      stepMap={new Map()}
      repoMap={new Map()}
      liveTasksById={liveTasksById}
      onTaskSelect={vi.fn()}
    />
  );
}

afterEach(cleanup);

describe("command panel task activity icons", () => {
  // @covers AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.1
  it("shows the sidebar running icon for an active task result", () => {
    render(
      taskResults([
        {
          ...task("task-running", "Running task"),
          primary_session_state: "RUNNING",
        },
      ]),
    );

    expect(screen.getByTestId("task-state-running")).toBeTruthy();
    expect(screen.getByRole("img", { name: "In progress" })).toBeTruthy();
  });

  // @covers AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.2
  it("shows the sidebar idle icon for a task that is not running", () => {
    render(taskResults([{ ...task("task-idle", "Idle task"), state: "TODO" }]));

    expect(screen.getByTestId("task-state-backlog")).toBeTruthy();
    expect(screen.getByRole("img", { name: "To do" })).toBeTruthy();
  });

  // @covers AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.3
  it("reconciles a loaded result with a newer running task update", () => {
    const loadedTask = { ...task("task-live", "Live task"), state: "TODO" as const };
    const liveTask: CommandPanelLiveTask = {
      id: loadedTask.id,
      workflowId: loadedTask.workflow_id,
      workflowStepId: loadedTask.workflow_step_id,
      title: loadedTask.title,
      description: loadedTask.description,
      position: loadedTask.position,
      state: "IN_PROGRESS",
      primarySessionState: "RUNNING",
      updatedAt: "2026-08-24T09:01:00Z",
    };

    const view = render(taskResults([loadedTask]));
    expect(screen.getByTestId("task-state-backlog")).toBeTruthy();

    view.rerender(taskResults([loadedTask], new Map([[loadedTask.id, liveTask]])));

    expect(screen.getByTestId("task-state-running")).toBeTruthy();
  });

  it("does not let an older live projection regress a newer search result", () => {
    const loadedTask = {
      ...task("task-fresh", "Fresh task"),
      state: "TODO" as const,
      updated_at: "2026-08-24T09:02:00Z",
    };
    const staleLiveTask: CommandPanelLiveTask = {
      id: loadedTask.id,
      workflowId: loadedTask.workflow_id,
      workflowStepId: loadedTask.workflow_step_id,
      title: loadedTask.title,
      position: loadedTask.position,
      state: "IN_PROGRESS",
      primarySessionState: "RUNNING",
      updatedAt: "2026-08-24T09:01:00Z",
    };

    render(taskResults([loadedTask], new Map([[loadedTask.id, staleLiveTask]])));

    expect(screen.getByTestId("task-state-backlog")).toBeTruthy();
    expect(screen.queryByTestId("task-state-running")).toBeNull();
  });
});
