import { describe, expect, it } from "vitest";
import { buildTaskTreeNodes, getExpandableTaskIds, type FlatTaskNode } from "./use-tasks-tree";
import type {
  OfficeTask,
  TaskFilterState,
  TaskSortDir,
  TaskSortField,
} from "@/lib/state/slices/office/types";

const FILTERS: TaskFilterState = {
  statuses: [],
  priorities: [],
  assigneeIds: [],
  projectIds: [],
  search: "",
};

const STATUS_ORDER = {
  backlog: 0,
  todo: 1,
  in_progress: 2,
  in_review: 3,
  blocked: 4,
  done: 5,
  cancelled: 6,
};

const PRIORITY_ORDER = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3,
  none: 4,
};

function task(id: string, title: string, parentId?: string): OfficeTask {
  return {
    id,
    workspaceId: "workspace-1",
    identifier: id.toUpperCase(),
    title,
    status: "todo",
    priority: "none",
    parentId,
    createdAt: "2026-01-01T00:00:00.000Z",
    updatedAt: "2026-01-01T00:00:00.000Z",
  };
}

function buildNodes(
  tasks: OfficeTask[],
  expandedIds: Set<string>,
  sortField: TaskSortField = "title",
  sortDir: TaskSortDir = "asc",
): FlatTaskNode[] {
  return buildTaskTreeNodes({
    tasks,
    filters: FILTERS,
    sortField,
    sortDir,
    nestingEnabled: true,
    expandedIds,
    statusOrder: STATUS_ORDER,
    priorityOrder: PRIORITY_ORDER,
  });
}

describe("office task tree", () => {
  it("marks parents with subtasks as expandable by default", () => {
    const parent = task("parent", "Parent");
    const child = task("child", "Child", parent.id);

    expect([...getExpandableTaskIds([parent, child])]).toEqual([parent.id]);
  });

  it("renders expanded subtasks nested under their parent", () => {
    const parent = task("parent", "Parent");
    const child = task("child", "Child", parent.id);

    const nodes = buildNodes([parent, child], new Set([parent.id]));

    expect(nodes.map((node) => [node.task.id, node.level, node.hasChildren])).toEqual([
      [parent.id, 0, true],
      [child.id, 1, false],
    ]);
  });

  it("does not promote collapsed subtasks to top-level rows", () => {
    const parent = task("parent", "Parent");
    const child = task("child", "Child", parent.id);

    const nodes = buildNodes([parent, child], new Set());

    expect(nodes.map((node) => node.task.id)).toEqual([parent.id]);
  });

  it("keeps filtered orphans visible at the top level", () => {
    const child = task("child", "Child", "missing-parent");

    const nodes = buildNodes([child], new Set());

    expect(nodes.map((node) => [node.task.id, node.level, node.hasChildren])).toEqual([
      [child.id, 0, false],
    ]);
  });

  it("keeps visible orphan subtrees indented when an ancestor is filtered out", () => {
    const parent = task("parent", "Hidden parent", "missing-grandparent");
    const child = task("child", "Visible child", parent.id);
    const grandchild = task("grandchild", "Visible grandchild", child.id);

    const nodes = buildTaskTreeNodes({
      tasks: [parent, child, grandchild],
      filters: { ...FILTERS, search: "Visible" },
      sortField: "title",
      sortDir: "asc",
      nestingEnabled: true,
      expandedIds: new Set([child.id]),
      statusOrder: STATUS_ORDER,
      priorityOrder: PRIORITY_ORDER,
    });

    expect(nodes.map((node) => [node.task.id, node.level, node.hasChildren])).toEqual([
      [child.id, 1, true],
      [grandchild.id, 2, false],
    ]);
  });
});
