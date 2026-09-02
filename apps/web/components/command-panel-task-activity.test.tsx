import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CommandPanelLiveTask } from "@/lib/commands/task-result-activity";
import type { Task } from "@/lib/types/http";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";

const RUNNING_ICON_TEST_ID = "task-state-running";
const BACKLOG_ICON_TEST_ID = "task-state-backlog";
const CURRENT_STEP_ID = "step-current";
const LIVE_UPDATED_AT = "2026-08-24T09:01:00Z";

vi.mock("@kandev/ui/command", () => ({
  CommandEmpty: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  CommandGroup: ({ children }: { children: ReactNode }) => <section>{children}</section>,
  CommandItem: ({
    children,
    className,
    onSelect,
  }: {
    children: ReactNode;
    className?: string;
    onSelect?: () => void;
  }) => (
    <div role="option" className={className} onClick={onSelect}>
      {children}
    </div>
  ),
  CommandShortcut: ({ children }: { children: ReactNode }) => <span>{children}</span>,
}));

vi.mock("@kandev/ui/badge", () => ({
  Badge: ({ children, className }: { children: ReactNode; className?: string }) => (
    <span className={className}>{children}</span>
  ),
}));

import { CommandsListContent } from "./command-panel-results";

function task(id: string, title: string, overrides: Partial<Task> = {}): Task {
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
    ...overrides,
  };
}

function taskResults(
  tasks: Task[],
  liveTasksById = new Map<string, CommandPanelLiveTask>(),
  lastStepIdByWorkflowId = new Map<string, string>(),
  stepMap = new Map<string, { name: string; color: string }>(),
  onTaskSelect = vi.fn(),
) {
  return (
    <CommandsListContent
      commands={[]}
      grouped={[]}
      search="task"
      onSelect={vi.fn()}
      taskResults={tasks}
      isSearching={false}
      stepMap={stepMap}
      repoMap={new Map()}
      liveTasksById={liveTasksById}
      lastStepIdByWorkflowId={lastStepIdByWorkflowId}
      onTaskSelect={onTaskSelect}
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

    expect(screen.getByTestId(RUNNING_ICON_TEST_ID)).toBeTruthy();
    expect(screen.getByRole("img", { name: "In progress" })).toBeTruthy();
  });

  // @covers AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.2
  it("shows the sidebar idle icon for a task that is not running", () => {
    render(taskResults([{ ...task("task-idle", "Idle task"), state: "TODO" }]));

    expect(screen.getByTestId(BACKLOG_ICON_TEST_ID)).toBeTruthy();
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
      updatedAt: LIVE_UPDATED_AT,
    };

    const view = render(taskResults([loadedTask]));
    expect(screen.getByTestId(BACKLOG_ICON_TEST_ID)).toBeTruthy();

    view.rerender(taskResults([loadedTask], new Map([[loadedTask.id, liveTask]])));

    expect(screen.getByTestId(RUNNING_ICON_TEST_ID)).toBeTruthy();
  });

  // @covers AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.8
  it("does not let an older live projection regress a newer search result", () => {
    const loadedTask = {
      ...task("task-fresh", "Fresh task"),
      workflow_step_id: CURRENT_STEP_ID,
      state: "TODO" as const,
      updated_at: "2026-08-24T09:02:00Z",
    };
    const staleLiveTask: CommandPanelLiveTask = {
      id: loadedTask.id,
      workflowId: loadedTask.workflow_id,
      workflowStepId: "step-stale",
      title: loadedTask.title,
      position: loadedTask.position,
      state: "IN_PROGRESS",
      primarySessionState: "RUNNING",
      updatedAt: LIVE_UPDATED_AT,
    };

    render(
      taskResults(
        [loadedTask],
        new Map([[loadedTask.id, staleLiveTask]]),
        new Map(),
        new Map([
          [CURRENT_STEP_ID, { name: "Review", color: "bg-purple-500" }],
          ["step-stale", { name: "Stale step", color: "bg-slate-500" }],
        ]),
      ),
    );

    expect(screen.getByTestId(BACKLOG_ICON_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
    expect(screen.getByText("Review")).toBeTruthy();
    expect(screen.queryByText("Stale step")).toBeNull();
  });

  // @covers AC-UI-COMMAND-PANEL-TASK-ACTIVITY-001.8
  it("shows the workflow step from a newer live task placement", () => {
    const loadedTask = {
      ...task("task-moved", "Moved task"),
      workflow_step_id: "step-old",
      updated_at: "2026-08-24T09:00:00Z",
    };
    const liveTask: CommandPanelLiveTask = {
      id: loadedTask.id,
      workflowId: loadedTask.workflow_id,
      workflowStepId: CURRENT_STEP_ID,
      title: loadedTask.title,
      position: loadedTask.position,
      state: loadedTask.state,
      updatedAt: LIVE_UPDATED_AT,
    };

    render(
      taskResults(
        [loadedTask],
        new Map([[loadedTask.id, liveTask]]),
        new Map(),
        new Map([
          ["step-old", { name: "In Progress", color: "bg-blue-500" }],
          [CURRENT_STEP_ID, { name: "Review", color: "bg-purple-500" }],
        ]),
      ),
    );

    expect(screen.getByText("Review")).toBeTruthy();
  });
});

describe("command panel task activity icon edge cases", () => {
  it("keeps the workflow-complete icon for a task on the final step", () => {
    render(
      taskResults(
        [{ ...task("task-complete", "Complete task"), state: "REVIEW" }],
        new Map(),
        new Map([["workflow-1", "step-1"]]),
      ),
    );

    expect(screen.getByTestId("task-state-workflow-complete")).toBeTruthy();
    expect(screen.getByRole("img", { name: "Completed" })).toBeTruthy();
  });

  it("treats a current live activity clear as authoritative", () => {
    const loadedTask = {
      ...task("task-cleared", "Cleared task"),
      state: "TODO" as const,
      foreground_activity: "generating" as const,
    };
    const liveTask: CommandPanelLiveTask = {
      id: loadedTask.id,
      workflowId: loadedTask.workflow_id,
      workflowStepId: loadedTask.workflow_step_id,
      title: loadedTask.title,
      position: loadedTask.position,
      state: "TODO",
      primarySessionState: "IDLE",
      foregroundActivity: null,
      updatedAt: LIVE_UPDATED_AT,
    };

    render(taskResults([loadedTask], new Map([[loadedTask.id, liveTask]])));

    expect(screen.getByTestId(BACKLOG_ICON_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(RUNNING_ICON_TEST_ID)).toBeNull();
  });

  it("accepts a legacy live projection without an updated timestamp", () => {
    const loadedTask = { ...task("task-legacy", "Legacy task"), state: "TODO" as const };
    const liveTask: CommandPanelLiveTask = {
      id: loadedTask.id,
      workflowId: loadedTask.workflow_id,
      workflowStepId: loadedTask.workflow_step_id,
      title: loadedTask.title,
      position: loadedTask.position,
      state: "IN_PROGRESS",
      primarySessionState: "RUNNING",
    };

    render(taskResults([loadedTask], new Map([[loadedTask.id, liveTask]])));

    expect(screen.getByTestId(RUNNING_ICON_TEST_ID)).toBeTruthy();
  });
});

describe("command panel archived task results", () => {
  // @covers AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.1, AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.2,
  // AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.3, AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.4,
  // AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.5
  it("uses archived_at for the archive cues instead of terminal state", () => {
    const archivedInProgress = task("archived-in-progress", "Archived in progress", {
      state: "IN_PROGRESS",
      archived_at: "2026-08-24T09:05:00Z",
    });
    const unarchivedCompleted = task("unarchived-completed", "Unarchived completed", {
      state: "COMPLETED",
    });

    render(
      taskResults(
        [archivedInProgress, unarchivedCompleted],
        new Map(),
        new Map(),
        new Map([["step-1", { name: "In Progress", color: "bg-blue-500" }]]),
      ),
    );

    const archivedBadge = screen.getByText("Archived", { exact: true });
    const archivedRow = archivedBadge.closest('[role="option"]');
    const archivedTitle = screen.getByText("Archived in progress", { exact: true });

    expect(archivedRow).not.toBeNull();
    expect(archivedRow!.className).not.toContain("opacity-60");
    expect(archivedRow!.contains(screen.getByText("In Progress", { exact: true }))).toBe(false);
    expect(archivedTitle.className).toContain("text-muted-foreground");
    expect(archivedBadge.className).toContain("text-muted-foreground");
    expect(screen.getByTestId("task-state-archived")).toBeTruthy();
    expect(screen.getByRole("img", { name: "Archived" })).toBeTruthy();
  });

  // @covers AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.7
  it("keeps archived result selection on the existing row action", () => {
    const archivedTask = task("archived-task", "Archived task", {
      state: "IN_PROGRESS",
      archived_at: "2026-08-24T09:05:00Z",
    });
    const onTaskSelect = vi.fn();

    render(taskResults([archivedTask], new Map(), new Map(), new Map(), onTaskSelect));

    fireEvent.click(screen.getByRole("option"));

    expect(onTaskSelect).toHaveBeenCalledWith(archivedTask);
  });
});
