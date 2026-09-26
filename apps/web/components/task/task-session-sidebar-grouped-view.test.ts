import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { TaskSwitcherItem } from "./task-switcher-types";

const view = {
  id: "view-1",
  name: "By workflow",
  filters: [],
  sort: { key: "title" as const, direction: "asc" as const },
  group: "workflow" as const,
  collapsedGroups: [],
};

const prefs = {
  pinnedTaskIds: [],
  orderedTaskIds: [],
  subtaskOrderByParentId: {},
  togglePinnedTask: vi.fn(),
  handleReorderGroup: vi.fn(),
  handleReorderSubtasks: vi.fn(),
};

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ i18n: { language: "en" } }),
}));

vi.mock("@/hooks/domains/sidebar/use-effective-sidebar-view", () => ({
  useEffectiveSidebarView: () => view,
}));

vi.mock("@/hooks/domains/sidebar/use-sidebar-task-prefs", () => ({
  useSidebarTaskPrefs: () => prefs,
}));

import { groupSidebarTaskPage, useGroupedSidebarView } from "./task-session-sidebar-grouped-view";

const WORKFLOW_A = "Workflow A";
const WORKFLOW_A_ID = "wf-a";
const PARENT_ID = "parent";
const CHILD_ID = "child";
const OFF_PAGE_PARENT_ID = "off-page-parent";
const PARENT_TITLE = "Parent";

function task(id: string, workflowId: string): TaskSwitcherItem {
  return { id, title: id, state: "IN_PROGRESS", workflowId, workflowName: workflowId };
}

describe("useGroupedSidebarView", () => {
  it("preserves an unaffected group reference when a task in another group updates", () => {
    const taskA = task("Task A", WORKFLOW_A);
    const taskB = task("Task B", "Workflow B");
    const hook = renderHook(({ tasks }) => useGroupedSidebarView(tasks), {
      initialProps: { tasks: [taskA, taskB] },
    });
    const unaffectedGroup = hook.result.current.grouped.groups.find(
      (group) => group.key === "Workflow B",
    );

    hook.rerender({ tasks: [{ ...taskA, title: "Task A updated" }, { ...taskB }] });

    expect(hook.result.current.grouped.groups[1]).toBe(unaffectedGroup);
  });
});

describe("groupSidebarTaskPage", () => {
  it("uses server entry order and keeps parent context at a page boundary", () => {
    const earlier = task("earlier", WORKFLOW_A);
    const later = { ...task("later", WORKFLOW_A), parentTaskId: OFF_PAGE_PARENT_ID };
    const grouped = groupSidebarTaskPage(
      [later, earlier],
      [
        {
          kind: "group",
          group_key: WORKFLOW_A_ID,
          group_label: WORKFLOW_A,
          matching_count: 101,
          continuation: true,
        },
        { kind: "continuation", parent_id: OFF_PAGE_PARENT_ID, parent_title: PARENT_TITLE },
        {
          kind: "task",
          task_id: "later",
          group_key: WORKFLOW_A_ID,
          parent_id: OFF_PAGE_PARENT_ID,
          parent_title: PARENT_TITLE,
        },
        { kind: "task", task_id: "earlier", group_key: WORKFLOW_A_ID },
      ],
      "workflow",
    );

    expect(grouped.groups[0]).toMatchObject({
      key: WORKFLOW_A_ID,
      matchingCount: 101,
      isContinuation: true,
    });
    expect(grouped.groups[0].tasks.map((item) => item.id)).toEqual(["later", "earlier"]);
    expect(grouped.groups[0].tasks[0]).toMatchObject({
      parentTaskId: OFF_PAGE_PARENT_ID,
      continuationParentTitle: PARENT_TITLE,
    });
    expect(grouped.subTasksByParentId.size).toBe(0);
  });

  it("nests children when their parent is on the same page", () => {
    const parent = task(PARENT_ID, WORKFLOW_A);
    const child = { ...task(CHILD_ID, WORKFLOW_A), parentTaskId: PARENT_ID };
    const grouped = groupSidebarTaskPage(
      [child, parent],
      [
        { kind: "group", group_key: WORKFLOW_A_ID, group_label: WORKFLOW_A },
        { kind: "task", task_id: PARENT_ID, group_key: WORKFLOW_A_ID },
        {
          kind: "task",
          task_id: CHILD_ID,
          group_key: WORKFLOW_A_ID,
          parent_id: PARENT_ID,
          depth: 1,
        },
      ],
      "workflow",
    );

    expect(grouped.groups[0].tasks.map((item) => item.id)).toEqual([PARENT_ID]);
    expect(grouped.subTasksByParentId.get(PARENT_ID)?.map((item) => item.id)).toEqual([CHILD_ID]);
  });

  it("does not nest a server-promoted root under a filtered parent from the page", () => {
    const parent = task(PARENT_ID, WORKFLOW_A);
    const child = { ...task(CHILD_ID, WORKFLOW_A), parentTaskId: PARENT_ID };
    const grouped = groupSidebarTaskPage(
      [parent, child],
      [
        { kind: "group", group_key: WORKFLOW_A_ID, group_label: WORKFLOW_A },
        {
          kind: "task",
          task_id: CHILD_ID,
          group_key: WORKFLOW_A_ID,
          parent_id: PARENT_ID,
          depth: 0,
        },
      ],
      "workflow",
    );

    expect(grouped.groups[0].tasks.map((item) => item.id)).toEqual([CHILD_ID]);
    expect(grouped.subTasksByParentId.size).toBe(0);
  });
});
