import type { ReactElement, ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { GroupedSidebarList } from "@/lib/sidebar/apply-view";
import type { TaskSwitcherItem } from "./task-switcher-types";

const renderCounts = vi.hoisted(() => ({
  groups: new Map<string, number>(),
  nodes: new Map<string, number>(),
  rows: new Map<string, number>(),
}));

function increment(counts: Map<string, number>, key: string) {
  counts.set(key, (counts.get(key) ?? 0) + 1);
}

vi.mock("./task-item", () => ({
  TaskItem: ({ title, onClick }: { title: string; onClick?: () => void }) => {
    increment(renderCounts.rows, title);
    return <button onClick={onClick}>{title}</button>;
  },
}));

vi.mock("./task-switcher-context-menu", () => ({
  TaskItemWithContextMenu: ({ children }: { children: ReactElement }) => children,
}));

vi.mock("./task-switcher-subtask-dnd", () => ({
  SortableTaskLevel: ({
    tasks,
    renderNode,
  }: {
    tasks: TaskSwitcherItem[];
    renderNode: (task: TaskSwitcherItem, isDraggable: boolean) => ReactNode;
  }) => <>{tasks.map((task) => renderNode(task, false))}</>,
  SortableTaskNode: ({
    taskId,
    handle,
    nested,
  }: {
    taskId: string;
    handle: ReactNode;
    nested?: ReactNode;
  }) => {
    increment(renderCounts.nodes, taskId);
    return (
      <>
        {handle}
        {nested}
      </>
    );
  },
  TaskTreeDndGroup: ({
    groupTasks,
    children,
  }: {
    groupTasks: TaskSwitcherItem[];
    children: (nestTargetIds: Set<string>, activeDragId: string | null) => ReactNode;
  }) => {
    increment(renderCounts.groups, groupTasks[0]?.workflowId ?? "empty");
    return <>{children(new Set(), null)}</>;
  },
  NestDropZone: () => null,
}));

import { TaskSwitcher } from "./task-switcher";

const onSelectTask = vi.fn();
const onToggleSubtasks = vi.fn();
const TASK_A = "Task A";
const TASK_A_UPDATED = "Task A updated";
const TASK_B = "Task B";
const WORKFLOW_A = "Workflow A";
const WORKFLOW_B = "Workflow B";

function task(id: string, workflowId: string): TaskSwitcherItem {
  return { id, title: id, state: "IN_PROGRESS", workflowId, workflowName: workflowId };
}

function groupedByWorkflow(tasks: TaskSwitcherItem[]): GroupedSidebarList {
  return {
    groups: tasks.map((item) => ({
      key: item.workflowId!,
      label: item.workflowName!,
      tasks: [item],
    })),
    subTasksByParentId: new Map(),
    groupKey: "workflow",
  };
}

function groupedTogether(tasks: TaskSwitcherItem[]): GroupedSidebarList {
  return {
    groups: [{ key: "__all__", label: "All", tasks }],
    subTasksByParentId: new Map(),
    groupKey: "none",
  };
}

function groupedTrees(
  entries: Array<{ root: TaskSwitcherItem; child: TaskSwitcherItem }>,
  groupKey: "workflow" | "none",
): GroupedSidebarList {
  return {
    groups:
      groupKey === "workflow"
        ? entries.map(({ root }) => ({
            key: root.workflowId!,
            label: root.workflowName!,
            tasks: [root],
          }))
        : [{ key: "__all__", label: "All", tasks: entries.map(({ root }) => root) }],
    subTasksByParentId: new Map(entries.map(({ root, child }) => [root.id, [child]])),
    groupKey,
  };
}

function switcher(grouped: GroupedSidebarList, selectTask = onSelectTask) {
  return (
    <TaskSwitcher
      grouped={grouped}
      activeTaskId={null}
      selectedTaskId={null}
      onSelectTask={selectTask}
      onToggleSubtasks={onToggleSubtasks}
    />
  );
}

beforeEach(() => {
  renderCounts.groups.clear();
  renderCounts.nodes.clear();
  renderCounts.rows.clear();
});

afterEach(() => cleanup());

describe("TaskSwitcher render stability", () => {
  it("does not rerender an unaffected expanded group when another group task updates", () => {
    const taskA = task(TASK_A, WORKFLOW_A);
    const taskB = task(TASK_B, WORKFLOW_B);
    const initial = groupedByWorkflow([taskA, taskB]);
    const view = render(switcher(initial));
    const updated = groupedByWorkflow([{ ...taskA, title: TASK_A_UPDATED }, { ...taskB }]);

    view.rerender(switcher(updated));

    expect(renderCounts.groups.get(WORKFLOW_B)).toBe(1);
  });

  it("does not rerender an unaffected row beside an updated task", () => {
    const taskA = task(TASK_A, WORKFLOW_A);
    const taskB = task(TASK_B, WORKFLOW_A);
    const initial = groupedTogether([taskA, taskB]);
    const view = render(switcher(initial));
    const updated = groupedTogether([{ ...taskA, title: TASK_A_UPDATED }, { ...taskB }]);

    view.rerender(switcher(updated));

    expect({
      updated: renderCounts.rows.get(TASK_A_UPDATED),
      unaffected: renderCounts.rows.get(TASK_B),
    }).toEqual({ updated: 1, unaffected: 1 });
  });

  it("does not rerender an unaffected row when task-derived handlers are recreated", () => {
    const taskA = task(TASK_A, WORKFLOW_A);
    const taskB = task(TASK_B, WORKFLOW_A);
    const initial = groupedTogether([taskA, taskB]);
    const view = render(switcher(initial, vi.fn()));
    const updated = groupedTogether([{ ...taskA, title: TASK_A_UPDATED }, { ...taskB }]);

    view.rerender(switcher(updated, vi.fn()));

    expect(renderCounts.rows.get(TASK_B)).toBe(1);
  });

  it("routes a skipped row action to the latest committed handler", () => {
    const taskA = task(TASK_A, WORKFLOW_A);
    const taskB = task(TASK_B, WORKFLOW_A);
    const initial = groupedTogether([taskA, taskB]);
    const view = render(switcher(initial, vi.fn()));
    const latestHandler = vi.fn();

    view.rerender(
      switcher(groupedTogether([{ ...taskA, title: TASK_A_UPDATED }, { ...taskB }]), latestHandler),
    );
    fireEvent.click(screen.getByRole("button", { name: TASK_B }));

    expect(latestHandler).toHaveBeenCalledWith(TASK_B);
  });
});

describe("TaskSwitcher nested render stability", () => {
  it("does not rerender an unaffected expanded group when another group subtask updates", () => {
    const parentA = task("Parent A", WORKFLOW_A);
    const childA = { ...task("Child A", WORKFLOW_A), parentTaskId: parentA.id };
    const parentB = task("Parent B", WORKFLOW_B);
    const childB = { ...task("Child B", WORKFLOW_B), parentTaskId: parentB.id };
    const initial = groupedTrees(
      [
        { root: parentA, child: childA },
        { root: parentB, child: childB },
      ],
      "workflow",
    );
    const view = render(switcher(initial));
    const updated = groupedTrees(
      [
        { root: { ...parentA }, child: { ...childA, title: "Child A updated" } },
        { root: { ...parentB }, child: { ...childB } },
      ],
      "workflow",
    );

    view.rerender(switcher(updated));

    expect(renderCounts.groups.get(WORKFLOW_B)).toBe(1);
  });

  it("does not rerender an unaffected expandable row beside an updated subtask", () => {
    const parentA = task("Parent A", WORKFLOW_A);
    const childA = { ...task("Child A", WORKFLOW_A), parentTaskId: parentA.id };
    const parentB = task("Parent B", WORKFLOW_A);
    const childB = { ...task("Child B", WORKFLOW_A), parentTaskId: parentB.id };
    const initial = groupedTrees(
      [
        { root: parentA, child: childA },
        { root: parentB, child: childB },
      ],
      "none",
    );
    const view = render(switcher(initial));
    const updated = groupedTrees(
      [
        { root: { ...parentA }, child: { ...childA, title: "Child A updated" } },
        { root: { ...parentB }, child: { ...childB } },
      ],
      "none",
    );

    view.rerender(switcher(updated));

    expect(renderCounts.rows.get("Parent B")).toBe(1);
  });

  it("does not rebuild an unaffected sibling tree node when a task updates", () => {
    const taskA = task(TASK_A, WORKFLOW_A);
    const taskB = task(TASK_B, WORKFLOW_A);
    const initial = groupedTogether([taskA, taskB]);
    const view = render(switcher(initial));

    view.rerender(switcher(groupedTogether([{ ...taskA, title: TASK_A_UPDATED }, { ...taskB }])));

    expect(renderCounts.nodes.get(TASK_B)).toBe(1);
  });
});
