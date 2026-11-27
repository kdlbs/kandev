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
import { flattenVisibleSidebarTasks } from "@/lib/sidebar/visible-task-order";

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
  it("updates a flag note while preserving an unaffected group reference", () => {
    const taskA = { ...task("Task A", WORKFLOW_A), flagNote: "Initial note" };
    const taskB = task("Task B", "Workflow B");
    const hook = renderHook(({ tasks }) => useGroupedSidebarView(tasks), {
      initialProps: { tasks: [taskA, taskB] },
    });
    const unaffectedGroup = hook.result.current.grouped.groups.find(
      (group) => group.key === "Workflow B",
    );

    hook.rerender({ tasks: [{ ...taskA, flagNote: "Edited note" }, { ...taskB }] });

    expect(hook.result.current.grouped.groups[0].tasks[0].flagNote).toBe("Edited note");
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

describe("groupSidebarTaskPage child state", () => {
  it("applies saved sibling order to the bounded page immediately", () => {
    const parent = task(PARENT_ID, WORKFLOW_A);
    const first = { ...task("first", WORKFLOW_A), parentTaskId: PARENT_ID };
    const second = { ...task("second", WORKFLOW_A), parentTaskId: PARENT_ID };
    const grouped = groupSidebarTaskPage(
      [parent, first, second],
      [
        { kind: "group", group_key: WORKFLOW_A_ID, group_label: WORKFLOW_A },
        { kind: "task", task_id: PARENT_ID, group_key: WORKFLOW_A_ID },
        {
          kind: "task",
          task_id: "first",
          group_key: WORKFLOW_A_ID,
          parent_id: PARENT_ID,
          depth: 1,
        },
        {
          kind: "task",
          task_id: "second",
          group_key: WORKFLOW_A_ID,
          parent_id: PARENT_ID,
          depth: 1,
        },
      ],
      "workflow",
      { [PARENT_ID]: ["second", "first"] },
    );

    expect(grouped.subTasksByParentId.get(PARENT_ID)?.map((item) => item.id)).toEqual([
      "second",
      "first",
    ]);
  });

  it("retains server descendant counts for collapsed tasks", () => {
    const parent = task(PARENT_ID, WORKFLOW_A);
    const grouped = groupSidebarTaskPage(
      [parent],
      [
        { kind: "group", group_key: WORKFLOW_A_ID, group_label: WORKFLOW_A },
        { kind: "task", task_id: PARENT_ID, group_key: WORKFLOW_A_ID, subtask_count: 2 },
      ],
      "workflow",
    );

    expect(grouped.groups[0].tasks[0].subtaskCount).toBe(2);
    expect(grouped.subTasksByParentId.size).toBe(0);
  });
});

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.3
it("keeps repeated plugin trees independent while selecting canonical tasks once", () => {
  const reference = "plugin:acme:labels";
  const parent = task(PARENT_ID, WORKFLOW_A);
  const child = { ...task(CHILD_ID, WORKFLOW_A), parentTaskId: PARENT_ID };
  const grouped = groupSidebarTaskPage(
    [parent, child],
    ["a", "b"].flatMap((value) => [
      { kind: "group" as const, group_key: `${reference}:value:${value}`, group_label: value },
      {
        kind: "task" as const,
        task_id: PARENT_ID,
        group_key: `${reference}:value:${value}`,
        subtask_count: 1,
      },
      {
        kind: "task" as const,
        task_id: CHILD_ID,
        group_key: `${reference}:value:${value}`,
        parent_id: PARENT_ID,
        depth: 1,
      },
    ]),
    reference,
  );
  for (const group of grouped.groups) {
    expect(group.tasks.map((task) => task.id)).toEqual([PARENT_ID]);
    expect(group.subTasksByParentId?.get(PARENT_ID)?.map((task) => task.id)).toEqual([CHILD_ID]);
  }
  const visible = flattenVisibleSidebarTasks(grouped, [], []);
  expect(visible.taskIds).toEqual([PARENT_ID, CHILD_ID]);
  expect(visible.occurrences.map((row) => row.taskId)).toEqual([
    PARENT_ID,
    CHILD_ID,
    PARENT_ID,
    CHILD_ID,
  ]);
  expect(new Set(visible.occurrences.map((row) => row.key)).size).toBe(4);
  expect(flattenVisibleSidebarTasks(grouped, [], [PARENT_ID]).taskIds).toEqual([PARENT_ID]);
  expect(parent.subtaskCount).toBeUndefined();
});

it("does not attach a continuation child to another plugin group's on-page parent", () => {
  const reference = "plugin:acme:labels";
  const grouped = groupSidebarTaskPage(
    [task(PARENT_ID, WORKFLOW_A), task(CHILD_ID, WORKFLOW_A)],
    [
      { kind: "group", group_key: `${reference}:value:a`, group_label: "A" },
      { kind: "task", task_id: PARENT_ID, group_key: `${reference}:value:a` },
      { kind: "group", group_key: `${reference}:value:b`, group_label: "B" },
      {
        kind: "task",
        task_id: CHILD_ID,
        group_key: `${reference}:value:b`,
        parent_id: PARENT_ID,
        parent_title: "Parent",
        depth: 1,
      },
    ],
    reference,
  );
  expect(grouped.groups[1].tasks).toMatchObject([
    { id: CHILD_ID, continuationParentTitle: "Parent" },
  ]);
});

it("uses a provider-neutral label for an unassigned plugin group", () => {
  const reference = "plugin:acme:labels";
  const grouped = groupSidebarTaskPage(
    [],
    [{ kind: "group", group_key: reference + ":unassigned", group_label: "" }],
    reference,
  );

  expect(grouped.groups[0].label).toBe("Unassigned");
});
